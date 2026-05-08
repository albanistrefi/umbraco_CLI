package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/schema"
)

func RegisterMedia(root *cobra.Command, deps Dependencies) {
	media := &cobra.Command{Use: "media", Short: "Media asset operations"}

	media.AddCommand(mediaGet(deps))
	media.AddCommand(mediaRoot(deps))
	media.AddCommand(mediaChildren(deps))
	media.AddCommand(mediaSearch(deps))
	media.AddCommand(mediaURLs(deps))
	media.AddCommand(mediaCreate(deps))
	media.AddCommand(mediaCreateFolder(deps))
	media.AddCommand(mediaUpload(deps))
	media.AddCommand(mediaUpdate(deps))
	media.AddCommand(mediaMove(deps))
	media.AddCommand(mediaDelete(deps))
	media.AddCommand(mediaTrash(deps))

	root.AddCommand(media)
}

func mediaGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get <id>", Short: "Get media by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaRoot(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "root", Short: "Get root media items", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), "/media/root", api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaChildren(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "children <id>", Short: "Get child media items", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s/children", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	var skip int
	var take int

	cmd := &cobra.Command{Use: "search", Short: "Search media items", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			if query == "" {
				return fmt.Errorf("media search requires either --params or --query")
			}
			params = map[string]any{"query": query}
			if skip >= 0 {
				params["skip"] = skip
			}
			if take >= 0 {
				params["take"] = take
			}
		}

		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/item/media/search", opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: "/media/search", opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search parameters as JSON")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
}

func mediaURLs(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "urls <id>", Short: "Get media URLs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s/urls", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func mediaCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	var printTemplate bool
	cmd := &cobra.Command{Use: "create", Short: "Create media from JSON payload", RunE: func(cmd *cobra.Command, args []string) error {
		if printTemplate {
			return printResult(cmd, deps, schema.Templates["media.create"])
		}
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		if _, err := ensurePayloadID(body); err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/media", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print an annotated JSON skeleton; substitute placeholders before passing to --json")
	return cmd
}

func mediaUpload(deps Dependencies) *cobra.Command {
	var name string
	var mediaType string
	var parent string
	var propertyAlias string
	var dryRun bool

	cmd := &cobra.Command{Use: "upload <file>", Short: "Upload a file and create a media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		if err := requireValue("--type", mediaType); err != nil {
			return err
		}
		if name == "" {
			name = mediaNameFromPath(filePath)
		}
		if err := requireValue("--name", name); err != nil {
			return err
		}

		tempID, err := newUUIDv4()
		if err != nil {
			return fmt.Errorf("failed to generate temporary file id: %w", err)
		}
		mediaID, err := newUUIDv4()
		if err != nil {
			return fmt.Errorf("failed to generate media id: %w", err)
		}
		mediaTypeRef, err := resolveMediaTypeReference(context.Background(), deps.Client, mediaType)
		if err != nil {
			return err
		}

		uploadResult, err := deps.Client.MultipartPost(
			context.Background(),
			"/temporary-file",
			map[string]string{"id": tempID},
			"file",
			filePath,
			api.RequestOptions{DryRun: dryRun},
		)
		if err != nil {
			return err
		}

		body := map[string]any{
			"id":        mediaID,
			"name":      name,
			"mediaType": mediaTypeRef,
			"values": []any{map[string]any{
				"alias": propertyAlias,
				"value": map[string]any{"temporaryFileId": tempID},
			}},
		}
		if parent != "" {
			body["parent"] = map[string]any{"id": parent}
		}

		createResultRaw, err := deps.Client.Post(context.Background(), "/media", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, map[string]any{
			"id":            mediaID,
			"name":          name,
			"mediaType":     body["mediaType"],
			"temporaryFile": map[string]any{"id": tempID, "upload": uploadResult},
			"created":       createResult(createResultRaw, body),
		})
	}}

	cmd.Flags().StringVar(&name, "name", "", "Media item name (defaults to file name without extension)")
	cmd.Flags().StringVar(&mediaType, "type", "", "Media type id, alias, or built-in name: Image, SVG, File, or Folder")
	cmd.Flags().StringVar(&parent, "parent", "", "Target parent media ID")
	cmd.Flags().StringVar(&propertyAlias, "property", "umbracoFile", "File property alias")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaNameFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

func resolveMediaTypeReference(ctx context.Context, client *api.Client, value string) (map[string]any, error) {
	normalized := normalizeMediaTypeAlias(value)
	if isUUIDLike(normalized) {
		return map[string]any{"id": normalized}, nil
	}

	result, err := getWithFallback(
		ctx,
		client,
		getRequestCandidate{path: "/item/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": normalized, "skip": 0, "take": 100}}},
		getRequestCandidate{path: "/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": normalized, "skip": 0, "take": 100}}},
		getRequestCandidate{path: "/media-type", opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve media type %q: %w", value, err)
	}

	id := findMediaTypeID(result, normalized)
	if id == "" {
		return nil, fmt.Errorf("media type %q was not found; pass a media type ID with --type or ensure the alias/name exists", value)
	}
	return map[string]any{"id": id}, nil
}

func normalizeMediaTypeAlias(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image":
		return "Image"
	case "svg":
		return "SVG"
	case "file":
		return "File"
	case "folder":
		return "Folder"
	default:
		return strings.TrimSpace(value)
	}
}

func findMediaTypeID(result any, query string) string {
	for _, item := range resultItems(result) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if matchesMediaType(entry, query) {
			if id, ok := entry["id"].(string); ok && strings.TrimSpace(id) != "" {
				return id
			}
		}
	}
	return ""
}

func matchesMediaType(entry map[string]any, query string) bool {
	for _, key := range []string{"alias", "name"} {
		if value, ok := entry[key].(string); ok && strings.EqualFold(value, query) {
			return true
		}
	}
	return false
}

func resultItems(result any) []any {
	if payload, ok := result.(map[string]any); ok {
		if items, ok := payload["items"].([]any); ok {
			return items
		}
	}
	if items, ok := result.([]any); ok {
		return items
	}
	return nil
}

func isUUIDLike(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

func mediaCreateFolder(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var parent string
	var dryRun bool
	cmd := &cobra.Command{Use: "create-folder [name]", Short: "Create media folder", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if len(args) == 0 || args[0] == "" {
				return fmt.Errorf("create-folder requires [name] when --json is not provided")
			}
			if err := requireValue("--parent", parent); err != nil {
				return err
			}
			body = map[string]any{"name": args[0], "parent": map[string]any{"id": parent}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/media/folder", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create-folder payload as JSON")
	cmd.Flags().StringVar(&parent, "parent", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "update <id>", Short: "Update media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/media/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaMove(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{Use: "move <id>", Short: "Move media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if err := requireValue("--to", to); err != nil {
				return err
			}
			body = map[string]any{"target": map[string]any{"id": to}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/media/%s/move", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Move payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "delete <id>", Short: "Delete media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/media/%s", args[0]), api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "trash <id>", Short: "Move media item to recycle bin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/media/%s/move-to-recycle-bin", args[0]), map[string]any{}, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}
