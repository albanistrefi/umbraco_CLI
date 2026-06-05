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
		return printResult(cmd, deps, applyFieldsProjection(result, fields))
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaRoot(deps Dependencies) *cobra.Command {
	var fields string
	var skip, take int
	var triage readTriageOptions
	cmd := &cobra.Command{Use: "root", Short: "Get root media items (paginated; use --skip/--take to walk past the server page size)", RunE: func(cmd *cobra.Command, args []string) error {
		params := applyPaginationParams(nil, skip, take)
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/tree/media/root", opts: api.RequestOptions{Fields: fields, Params: params}},
			getRequestCandidate{path: "/media/root", opts: api.RequestOptions{Fields: fields, Params: params}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addPaginationFlags(cmd, &skip, &take)
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func mediaChildren(deps Dependencies) *cobra.Command {
	var fields string
	var skip, take int
	var triage readTriageOptions
	cmd := &cobra.Command{Use: "children <id>", Short: "Get child media items (paginated; use --skip/--take to walk past the server page size)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		treeParams := applyPaginationParams(map[string]any{"parentId": args[0]}, skip, take)
		legacyParams := applyPaginationParams(nil, skip, take)
		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{
				path: "/tree/media/children",
				opts: api.RequestOptions{Fields: fields, Params: treeParams},
			},
			getRequestCandidate{
				path: fmt.Sprintf("/media/%s/children", args[0]),
				opts: api.RequestOptions{Fields: fields, Params: legacyParams},
			},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyReadTriage(applyFieldsProjection(result, fields), triage))
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	addPaginationFlags(cmd, &skip, &take)
	addReadTriageFlags(cmd, &triage)
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
	var culture string
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
		mediaTypeInfo, err := resolveMediaTypeInfo(context.Background(), deps.Client, mediaType)
		if err != nil {
			return err
		}

		// The Management API contract: every media create body uses a variants[] envelope.
		// For invariant media types the variant's culture is JSON null; for culture-varying
		// types it carries the culture code. There is never a top-level "name" field.
		cultureExplicit := strings.TrimSpace(culture) != ""
		var resolvedCulture string
		switch {
		case !mediaTypeInfo.VariesByCulture:
			if cultureExplicit {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: --culture %q ignored; media type %q (alias=%s) does not vary by culture\n", culture, mediaType, mediaTypeInfo.Alias)
			}
			resolvedCulture = ""
		case cultureExplicit:
			resolvedCulture = strings.TrimSpace(culture)
		default:
			resolvedCulture, err = resolveDefaultCulture(context.Background(), deps.Client)
			if err != nil {
				return fmt.Errorf("media type %q varies by culture; pass --culture <code>", mediaType)
			}
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

		var variantCulture any
		if resolvedCulture != "" {
			variantCulture = resolvedCulture
		}
		body := map[string]any{
			"id":        mediaID,
			"mediaType": map[string]any{"id": mediaTypeInfo.ID},
			"variants": []any{map[string]any{
				"name":    name,
				"culture": variantCulture,
			}},
			"values": []any{map[string]any{
				"alias":   propertyAlias,
				"culture": variantCulture,
				"value":   map[string]any{"temporaryFileId": tempID},
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
	cmd.Flags().StringVar(&mediaType, "type", "", "Media type id, alias, or name")
	cmd.Flags().StringVar(&culture, "culture", "", "Culture code for culture-varying media types")
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

type mediaTypeInfo struct {
	ID              string
	Alias           string
	Name            string
	VariesByCulture bool
}

// mediaTypeFriendlyAliases maps short, human-friendly names to the canonical Umbraco aliases for
// the built-in media types. Lookup is case-insensitive on the key. Listed names that already match
// their canonical alias (Image, File, Folder) are included so future renames stay backwards
// compatible.
var mediaTypeFriendlyAliases = map[string]string{
	"image":   "Image",
	"file":    "File",
	"folder":  "Folder",
	"svg":     "umbracoMediaVectorGraphics",
	"audio":   "umbracoMediaAudio",
	"video":   "umbracoMediaVideo",
	"article": "umbracoMediaArticle",
}

func resolveMediaTypeInfo(ctx context.Context, client *api.Client, value string) (mediaTypeInfo, error) {
	normalized := strings.TrimSpace(value)
	if canonical, ok := mediaTypeFriendlyAliases[strings.ToLower(normalized)]; ok {
		normalized = canonical
	}

	if isUUIDLike(normalized) {
		return fetchMediaTypeDetail(ctx, client, normalized, value)
	}

	// Lightweight endpoints (search, tree, item) return models without an alias field; only
	// /media-type/{id} carries it. Collect candidate IDs from the cheap endpoints and verify
	// each against its full detail until we find a case-insensitive alias or name match.
	candidateIDs := collectMediaTypeCandidateIDs(ctx, client, normalized)
	var nameMatch mediaTypeInfo
	visited := make(map[string]struct{}, len(candidateIDs))
	for _, id := range candidateIDs {
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}

		info, err := fetchMediaTypeDetail(ctx, client, id, value)
		if err != nil {
			continue
		}
		if strings.EqualFold(info.Alias, normalized) {
			return info, nil
		}
		if nameMatch.ID == "" && strings.EqualFold(info.Name, normalized) {
			nameMatch = info
		}
	}
	if nameMatch.ID != "" {
		return nameMatch, nil
	}

	return mediaTypeInfo{}, fmt.Errorf("media type %q was not found; pass a media type ID with --type or ensure the alias/name exists", value)
}

// collectMediaTypeCandidateIDs gathers media type IDs to inspect for alias/name matching.
// Search results come first (Examine matches both name and alias index, so the SVG type can
// surface from a query like "umbracoMediaVectorGraphics"), followed by the full tree-root
// listing as a fallback for installs where search misses or returns nothing.
func collectMediaTypeCandidateIDs(ctx context.Context, client *api.Client, query string) []string {
	var ids []string

	if searchResult, err := getWithFallback(
		ctx,
		client,
		getRequestCandidate{path: "/item/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": query, "skip": 0, "take": 50}}},
		getRequestCandidate{path: "/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": query, "skip": 0, "take": 50}}},
	); err == nil {
		ids = append(ids, mediaTypeIDsFromResult(searchResult)...)
	}

	if treeResult, err := client.Get(ctx, "/tree/media-type/root", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 500}}); err == nil {
		ids = append(ids, mediaTypeIDsFromResult(treeResult)...)
	}

	return ids
}

func mediaTypeIDsFromResult(result any) []string {
	var ids []string
	for _, item := range resultItems(result) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := entry["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func fetchMediaTypeDetail(ctx context.Context, client *api.Client, id string, originalValue string) (mediaTypeInfo, error) {
	detail, err := client.Get(ctx, fmt.Sprintf("/media-type/%s", id), api.RequestOptions{})
	if err != nil {
		return mediaTypeInfo{}, fmt.Errorf("failed to inspect media type %q (%s): %w", originalValue, id, err)
	}
	if info := mediaTypeInfoFromAny(detail); info.ID != "" {
		return info, nil
	}
	return mediaTypeInfo{ID: id}, nil
}

func findMediaTypeInfo(result any, query string) mediaTypeInfo {
	var nameMatch mediaTypeInfo
	for _, item := range resultItems(result) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		info := mediaTypeInfoFromMap(entry)
		if info.ID == "" {
			continue
		}
		if strings.EqualFold(info.Alias, query) {
			return info
		}
		if nameMatch.ID == "" && strings.EqualFold(info.Name, query) {
			nameMatch = info
		}
	}
	return nameMatch
}

func mediaTypeInfoFromAny(value any) mediaTypeInfo {
	entry, ok := value.(map[string]any)
	if !ok {
		return mediaTypeInfo{}
	}
	return mediaTypeInfoFromMap(entry)
}

func mediaTypeInfoFromMap(entry map[string]any) mediaTypeInfo {
	info := mediaTypeInfo{}
	if id, ok := entry["id"].(string); ok {
		info.ID = strings.TrimSpace(id)
	}
	if alias, ok := entry["alias"].(string); ok {
		info.Alias = strings.TrimSpace(alias)
	}
	if name, ok := entry["name"].(string); ok {
		info.Name = strings.TrimSpace(name)
	}
	if varies, ok := entry["variesByCulture"].(bool); ok {
		info.VariesByCulture = varies
	}
	return info
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

func resolveDefaultCulture(ctx context.Context, client *api.Client) (string, error) {
	result, err := getWithFallback(
		ctx,
		client,
		getRequestCandidate{path: "/server/configuration", opts: api.RequestOptions{}},
		getRequestCandidate{path: "/server/config", opts: api.RequestOptions{}},
	)
	if err != nil {
		return "", err
	}
	if culture := findStringByKeys(result, "defaultCulture", "defaultIsoCode", "defaultLanguageIsoCode"); culture != "" {
		return culture, nil
	}
	return "", fmt.Errorf("default culture not found")
}

func findStringByKeys(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := typed[key].(string); ok && strings.TrimSpace(raw) != "" {
				return strings.TrimSpace(raw)
			}
			if nested, ok := typed[key].(map[string]any); ok {
				if raw, ok := nested["isoCode"].(string); ok && strings.TrimSpace(raw) != "" {
					return strings.TrimSpace(raw)
				}
			}
		}
		for _, child := range typed {
			if result := findStringByKeys(child, keys...); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findStringByKeys(child, keys...); result != "" {
				return result
			}
		}
	}
	return ""
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
