package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	media.AddCommand(mediaReferences(deps))
	media.AddCommand(mediaReferencedDescendants(deps))
	media.AddCommand(mediaAreReferenced(deps))

	root.AddCommand(media)
}

func mediaGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <id>",
		Short: "Get media by ID",
		Path:  func(args []string) string { return api.JoinPath("/media/%s", args[0]) },
	})
}

func mediaRoot(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "root",
		Short: "Get root media items (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/media/root", opts: api.RequestOptions{Params: params}},
				{path: "/media/root", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func mediaChildren(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "children <id>",
		Short: "Get child media items (paginated; --skip/--take/--all)",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/media/children", opts: api.RequestOptions{Params: withParam(params, "parentId", args[0])}},
				{path: api.JoinPath("/media/%s/children", args[0]), opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func mediaSearch(deps Dependencies) *cobra.Command {
	return searchCommand(deps, searchSpec{
		Use:   "search",
		Short: "Search media items",
		Endpoints: func(params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/item/media/search", opts: api.RequestOptions{Params: params}},
				{path: "/media/search", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func mediaURLs(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "urls <id>", Short: "Get media URLs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := getWithFallback(
			cmd.Context(),
			deps.Client,
			getRequestCandidate{path: "/media/urls", opts: api.RequestOptions{Params: map[string]any{"id": args[0]}}},
			getRequestCandidate{path: api.JoinPath("/media/%s/urls", args[0]), opts: api.RequestOptions{}},
		)
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
		result, err := deps.Client.Post(cmd.Context(), "/media", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, createResult(result, body))
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	addDryRunFlag(cmd, &dryRun)
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
		ctx := cmd.Context()
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
		mediaTypeInfo, err := resolveMediaTypeInfo(ctx, deps.Client, mediaType)
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
			resolvedCulture, err = resolveDefaultCulture(ctx, deps.Client)
			if err != nil {
				return fmt.Errorf("media type %q varies by culture; pass --culture <code>", mediaType)
			}
		}

		uploadResult, err := deps.Client.MultipartPost(
			ctx,
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

		createResultRaw, err := deps.Client.Post(ctx, "/media", body, api.RequestOptions{DryRun: dryRun})
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
	addDryRunFlag(cmd, &dryRun)
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
	candidateIDs, lookupErr := collectMediaTypeCandidateIDs(ctx, client, normalized)
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

	// A failed lookup is a very different situation from a successful lookup
	// with no match — reporting "not found" while the server was down or auth
	// expired sends the caller chasing the wrong problem.
	if lookupErr != nil {
		return mediaTypeInfo{}, fmt.Errorf("could not resolve media type %q: %w", value, lookupErr)
	}
	return mediaTypeInfo{}, fmt.Errorf("media type %q was not found; pass a media type ID with --type or ensure the alias/name exists", value)
}

// collectMediaTypeCandidateIDs gathers media type IDs to inspect for alias/name matching.
// Search results come first (Examine matches both name and alias index, so the SVG type can
// surface from a query like "umbracoMediaVectorGraphics"), followed by the full tree-root
// listing as a fallback for installs where search misses or returns nothing. 404s mean the
// endpoint variant doesn't exist on this Umbraco version and are skipped; any other error is
// returned alongside whatever IDs were collected so the caller can distinguish "no match"
// from "lookup failed".
func collectMediaTypeCandidateIDs(ctx context.Context, client *api.Client, query string) ([]string, error) {
	var ids []string
	var lookupErr error

	noteErr := func(err error) {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return
		}
		if lookupErr == nil {
			lookupErr = err
		}
	}

	searchResult, err := getWithFallback(
		ctx,
		client,
		getRequestCandidate{path: "/item/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": query, "skip": 0, "take": 50}}},
		getRequestCandidate{path: "/media-type/search", opts: api.RequestOptions{Params: map[string]any{"query": query, "skip": 0, "take": 50}}},
	)
	if err == nil {
		ids = append(ids, mediaTypeIDsFromResult(searchResult)...)
	} else {
		noteErr(err)
	}

	treeResult, err := client.Get(ctx, "/tree/media-type/root", api.RequestOptions{Params: map[string]any{"skip": 0, "take": 500}})
	if err == nil {
		ids = append(ids, mediaTypeIDsFromResult(treeResult)...)
	} else {
		noteErr(err)
	}

	return ids, lookupErr
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
	detail, err := client.Get(ctx, api.JoinPath("/media-type/%s", id), api.RequestOptions{})
	if err != nil {
		return mediaTypeInfo{}, fmt.Errorf("failed to inspect media type %q (%s): %w", originalValue, id, err)
	}
	if info := mediaTypeInfoFromAny(detail); info.ID != "" {
		return info, nil
	}
	return mediaTypeInfo{ID: id}, nil
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
	cmd := &cobra.Command{
		Use:   "create-folder [name]",
		Short: "Create media folder",
		Long:  "Folders are regular media items of the built-in Folder type, so this resolves the Folder media type and POSTs /media with a variants envelope. --json passes a full media create payload through verbatim.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var body map[string]any
			if jsonPayload != "" {
				parsed, err := parsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				if len(args) == 0 || args[0] == "" {
					return fmt.Errorf("create-folder requires [name] when --json is not provided")
				}
				folderType, err := resolveMediaTypeInfo(ctx, deps.Client, "Folder")
				if err != nil {
					return fmt.Errorf("could not resolve the Folder media type: %w", err)
				}
				folderID, err := newUUIDv4()
				if err != nil {
					return fmt.Errorf("failed to generate media id: %w", err)
				}
				body = map[string]any{
					"id":        folderID,
					"mediaType": map[string]any{"id": folderType.ID},
					"variants":  []any{map[string]any{"name": args[0], "culture": nil}},
					"values":    []any{},
				}
				if strings.TrimSpace(parent) != "" {
					body["parent"] = map[string]any{"id": parent}
				}
			}
			result, err := deps.Client.Post(ctx, "/media", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, createResult(result, body))
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full media create payload as JSON (bypasses Folder-type resolution)")
	cmd.Flags().StringVar(&parent, "parent", "", "Target parent media ID (omit for a root-level folder)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func mediaUpdate(deps Dependencies) *cobra.Command {
	return updateCommand(deps, updateSpec{
		Use:   "update <id>",
		Short: "Update media item",
		Path:  func(args []string) string { return api.JoinPath("/media/%s", args[0]) },
	})
}

func mediaMove(deps Dependencies) *cobra.Command {
	return targetActionCommand(deps, targetActionSpec{
		Use:   "move <id>",
		Short: "Move media item",
		Candidates: func(args []string) []mutationCandidate {
			path := api.JoinPath("/media/%s/move", args[0])
			return []mutationCandidate{{method: "PUT", path: path}, {method: "POST", path: path}}
		},
		Verb: "moved",
	})
}

func mediaDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, "delete <id>", "Permanently delete a media item (use 'trash' for the recycle bin)", func(args []string) string {
		return api.JoinPath("/media/%s", args[0])
	})
}

func mediaTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "trash <id>", Short: "Move media item to recycle bin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path := api.JoinPath("/media/%s/move-to-recycle-bin", args[0])
		result, err := mutateWithFallback(cmd.Context(), deps.Client, map[string]any{}, api.RequestOptions{DryRun: dryRun},
			mutationCandidate{method: "PUT", path: path},
			mutationCandidate{method: "POST", path: path},
		)
		if err != nil {
			return err
		}
		return printMutationResult(cmd, deps, "trashed", result, dryRun)
	}}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func mediaReferences(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "references <id>",
		Short: "List items that reference this media item (paginated; --skip/--take/--all)",
		Long:  "Wraps GET /media/{id}/referenced-by. Same content-audit role as 'document references' for media assets.",
		Path:  func(args []string) string { return api.JoinPath("/media/%s/referenced-by", args[0]) },
	})
}

func mediaReferencedDescendants(deps Dependencies) *cobra.Command {
	return referencesCommand(deps, referencesSpec{
		Use:   "referenced-descendants <id>",
		Short: "List items that reference this media item or any of its descendants",
		Path:  func(args []string) string { return api.JoinPath("/media/%s/referenced-descendants", args[0]) },
	})
}

func mediaAreReferenced(deps Dependencies) *cobra.Command {
	return areReferencedCommand(deps, "media")
}
