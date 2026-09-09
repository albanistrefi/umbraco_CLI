package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// mediaInspect answers "which file is this?" in one read: file src and URL,
// extension, size, dimensions (raster from the umbracoWidth/umbracoHeight
// values, SVG viewBox from the file itself), and the remaining values —
// instead of digging through the values array and a separate urls call.
func mediaInspect(deps Dependencies) *cobra.Command {
	var propertyAlias string
	var noFetch bool
	cmd := &cobra.Command{
		Use:     "inspect <id>",
		Aliases: []string{"info"},
		Short:   "Summarize a media item: name, type, file src/URL, extension, size, dimensions",
		Long: "One-call view of what a media item points at. Combines GET /media/{id} with the public URL and flattens the file property (default umbracoFile) into file.src, file.url, file.extension, file.bytes. " +
			"Raster dimensions come from the umbracoWidth/umbracoHeight values; for SVGs the file is fetched and its viewBox/width/height attributes are reported (skip with --no-fetch). " +
			"Use before and after 'media replace-file' to confirm the swap, or 'media references <id>' to see which content uses the item.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			item, err := fetchObject(ctx, deps.Client, api.JoinPath("/media/%s", args[0]), api.RequestOptions{})
			if err != nil {
				return err
			}
			summary := summarizeMedia(item, propertyAlias)
			summary["id"] = args[0]

			if urls, err := mediaPublicURLs(ctx, deps.Client, args[0]); err == nil && len(urls) > 0 {
				summary["urls"] = urls
				if file, ok := summary["file"].(map[string]any); ok && file["url"] == nil {
					file["url"] = urls[0]
				}
			}

			if file, ok := summary["file"].(map[string]any); ok && !noFetch && strings.EqualFold(fmt.Sprint(file["extension"]), "svg") {
				if src := strings.TrimSpace(fmt.Sprint(file["src"])); src != "" && src != "<nil>" {
					content, _, err := deps.Client.GetBytes(ctx, src, api.RequestOptions{RawPath: true})
					if err == nil {
						for key, value := range svgAttributes(content) {
							file[key] = value
						}
						if file["bytes"] == nil {
							file["bytes"] = len(content)
						}
					} else {
						file["fetchError"] = err.Error()
					}
				}
			}
			return printResult(cmd, deps, summary)
		},
	}
	cmd.Flags().StringVar(&propertyAlias, "property", "umbracoFile", "File property alias to summarize")
	cmd.Flags().BoolVar(&noFetch, "no-fetch", false, "Do not download SVG files to read their viewBox")
	return cmd
}

// mediaDownload saves the file behind a media item to disk, byte for byte.
func mediaDownload(deps Dependencies) *cobra.Command {
	var propertyAlias string
	cmd := &cobra.Command{
		Use:     "download <id> <path>",
		Aliases: []string{"get-file"},
		Short:   "Download the file behind a media item to a local path",
		Long:    "Resolves the file property (default umbracoFile) and fetches the asset from the same host, writing it verbatim. If <path> is an existing directory (or ends with /), the server-side file name is used inside it.",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			item, err := fetchObject(ctx, deps.Client, api.JoinPath("/media/%s", args[0]), api.RequestOptions{})
			if err != nil {
				return err
			}
			value, ok := mediaFileValue(item, propertyAlias)
			if !ok {
				return fmt.Errorf("media item %s has no %q property; check the media type or pass --property", args[0], propertyAlias)
			}
			src := strings.TrimSpace(fmt.Sprint(value["src"]))
			if src == "" || src == "<nil>" {
				return fmt.Errorf("media item %s has no file behind %q", args[0], propertyAlias)
			}
			content, contentType, err := deps.Client.GetBytes(ctx, src, api.RequestOptions{RawPath: true})
			if err != nil {
				return err
			}
			target := args[1]
			if info, statErr := os.Stat(target); strings.HasSuffix(target, "/") || (statErr == nil && info.IsDir()) {
				target = filepath.Join(target, path.Base(src))
			}
			if dir := filepath.Dir(target); dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				return fmt.Errorf("failed to write %s: %w", target, err)
			}
			return printResult(cmd, deps, map[string]any{
				"id":          args[0],
				"property":    propertyAlias,
				"src":         src,
				"path":        target,
				"bytes":       len(content),
				"contentType": contentType,
			})
		},
	}
	cmd.Flags().StringVar(&propertyAlias, "property", "umbracoFile", "File property alias to download")
	return cmd
}

// summarizeMedia flattens a media item into the inspect shape.
func summarizeMedia(item map[string]any, propertyAlias string) map[string]any {
	summary := map[string]any{
		"mediaType": item["mediaType"],
		"isTrashed": item["isTrashed"],
	}
	if variants, ok := item["variants"].([]any); ok && len(variants) > 0 {
		if first, ok := variants[0].(map[string]any); ok {
			summary["name"] = first["name"]
			summary["createDate"] = first["createDate"]
			summary["updateDate"] = first["updateDate"]
		}
	}

	file := map[string]any{"property": propertyAlias}
	other := map[string]any{}
	var width, height any
	for _, raw := range mediaValues(item) {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		alias := fmt.Sprint(entry["alias"])
		switch alias {
		case propertyAlias:
			if value, ok := entry["value"].(map[string]any); ok {
				file["src"] = value["src"]
				// The crops array carries every crop *definition* of the data
				// type; only entries with coordinates are actual edits.
				if crops, ok := value["crops"].([]any); ok {
					edited := []any{}
					for _, rawCrop := range crops {
						if crop, ok := rawCrop.(map[string]any); ok && crop["coordinates"] != nil {
							edited = append(edited, crop)
						}
					}
					file["cropDefinitions"] = len(crops)
					if len(edited) > 0 {
						file["crops"] = edited
					}
				}
				if focal, ok := value["focalPoint"]; ok && focal != nil {
					file["focalPoint"] = focal
				}
			} else {
				file["src"] = entry["value"]
			}
		case "umbracoExtension":
			file["extension"] = entry["value"]
		case "umbracoBytes":
			file["bytes"] = numericValue(entry["value"])
		case "umbracoWidth":
			width = numericValue(entry["value"])
		case "umbracoHeight":
			height = numericValue(entry["value"])
		default:
			other[alias] = entry["value"]
		}
	}
	if width != nil || height != nil {
		file["width"] = width
		file["height"] = height
	}
	if file["extension"] == nil {
		if src := fmt.Sprint(file["src"]); src != "" && src != "<nil>" {
			file["extension"] = strings.TrimPrefix(path.Ext(src), ".")
		}
	}
	summary["file"] = file
	summary["values"] = len(mediaValues(item))
	if len(other) > 0 {
		summary["otherValues"] = other
	}
	return summary
}

func numericValue(value any) any {
	switch typed := value.(type) {
	case string:
		if n, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			if n == float64(int64(n)) {
				return int64(n)
			}
			return n
		}
		return typed
	default:
		return value
	}
}

// mediaPublicURLs returns the public URLs reported by /media/urls.
func mediaPublicURLs(ctx context.Context, client *api.Client, id string) ([]string, error) {
	result, err := getWithFallback(ctx, client,
		getRequestCandidate{path: "/media/urls", opts: api.RequestOptions{Params: map[string]any{"id": id}}},
		getRequestCandidate{path: api.JoinPath("/media/%s/urls", id), opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	urls := []string{}
	items, _ := result.([]any)
	for _, raw := range items {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		infos, _ := entry["urlInfos"].([]any)
		for _, rawInfo := range infos {
			if info, ok := rawInfo.(map[string]any); ok {
				if url := strings.TrimSpace(fmt.Sprint(info["url"])); url != "" && url != "<nil>" {
					urls = append(urls, url)
				}
			}
		}
	}
	return urls, nil
}

var svgAttributePattern = regexp.MustCompile(`(?is)<svg\b[^>]*>`)
var svgAttrValuePattern = regexp.MustCompile(`(?i)\b(viewBox|width|height)\s*=\s*["']([^"']*)["']`)

// svgAttributes pulls viewBox/width/height off the root <svg> element.
func svgAttributes(content []byte) map[string]any {
	attrs := map[string]any{}
	root := svgAttributePattern.Find(content)
	if root == nil {
		return attrs
	}
	for _, match := range svgAttrValuePattern.FindAllSubmatch(root, -1) {
		key := strings.ToLower(string(match[1]))
		value := strings.TrimSpace(string(match[2]))
		switch key {
		case "viewbox":
			attrs["viewBox"] = value
		case "width":
			attrs["svgWidth"] = value
		case "height":
			attrs["svgHeight"] = value
		}
	}
	return attrs
}
