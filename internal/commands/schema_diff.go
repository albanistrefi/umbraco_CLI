package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/config"
)

type schemaDiffFoundError struct{}

func (schemaDiffFoundError) Error() string {
	return "schema differences found"
}

// ExitCode implements the CLI's documented exit-code contract: 2 means the
// comparison ran cleanly and found real differences, so CI gates can tell
// "schemas diverged" apart from "the check itself failed".
func (schemaDiffFoundError) ExitCode() int { return 2 }

func schemaDiffCommand(deps cmdkit.Dependencies) *cobra.Command {
	var entityRaw string
	var include []string
	var exclude []string
	var exitZero bool

	cmd := &cobra.Command{
		Use:   "diff <envA> <envB>",
		Short: "Compare schema between two configured environments",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			entities, err := parseSchemaDiffEntities(entityRaw)
			if err != nil {
				return err
			}
			opts := schemaDiffOptions{
				Entities: entities,
				Include:  include,
				Exclude:  exclude,
			}

			left, err := fetchSchemaDiffEnvironment(cmd.Context(), "envA", args[0], entities, deps)
			if err != nil {
				return err
			}
			right, err := fetchSchemaDiffEnvironment(cmd.Context(), "envB", args[1], entities, deps)
			if err != nil {
				return err
			}

			report := computeSchemaDiff(args[0], args[1], left, right, opts)
			if err := printSchemaDiffReport(cmd, deps, report); err != nil {
				return err
			}
			if !report.Equal && !exitZero {
				return schemaDiffFoundError{}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&entityRaw, "entity", "", "Comma-separated entity kinds to compare: doctype,datatype,mediatype,membertype,template,language,dictionary (default: doctype,datatype)")
	cmd.Flags().StringArrayVar(&include, "include", nil, "Only include matching aliases/names; repeat or comma-separate")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Exclude matching aliases/names; repeat or comma-separate")
	cmd.Flags().BoolVar(&exitZero, "exit-zero", false, "Exit 0 even when schema differences are found")
	return cmd
}

func fetchSchemaDiffEnvironment(ctx context.Context, side string, label string, entities []schemaDiffEntityKind, deps cmdkit.Dependencies) ([]schemaDiffEntity, error) {
	cfg, err := config.LoadWithOptions(config.LoadOptions{Profile: label})
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", side, label, err)
	}
	httpClient := deps.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := api.NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	requested := func(kind schemaDiffEntityKind) bool {
		return schemaDiffEntityRequested(entities, kind)
	}
	// Reference maps translate server-assigned IDs into cross-environment
	// aliases before diffing. Datatype refs are needed by every property-
	// bearing type kind; type refs are needed for composition references.
	needsDatatypes := requested(schemaDiffDatatype) || requested(schemaDiffDoctype) || requested(schemaDiffMediatype) || requested(schemaDiffMembertype)

	raws := map[schemaDiffEntityKind][]map[string]any{}
	fetchKind := func(kind schemaDiffEntityKind, needed bool) error {
		if !needed {
			return nil
		}
		if _, done := raws[kind]; done {
			return nil
		}
		fetched, err := fetchSchemaDiffRawEntities(ctx, client, kind)
		if err != nil {
			return fmt.Errorf("%s %q %s fetch failed: %w", side, label, kind, err)
		}
		raws[kind] = fetched
		return nil
	}

	if err := fetchKind(schemaDiffDatatype, needsDatatypes); err != nil {
		return nil, err
	}
	for _, kind := range []schemaDiffEntityKind{schemaDiffDoctype, schemaDiffMediatype, schemaDiffMembertype, schemaDiffTemplate, schemaDiffLanguage, schemaDiffDictionary} {
		if err := fetchKind(kind, requested(kind)); err != nil {
			return nil, err
		}
	}

	refs := schemaDiffReferences{
		DataTypes:     schemaDiffIDAliasMap(schemaDiffDatatype, raws[schemaDiffDatatype]),
		DocumentTypes: schemaDiffIDAliasMap(schemaDiffDoctype, raws[schemaDiffDoctype]),
		MediaTypes:    schemaDiffIDAliasMap(schemaDiffMediatype, raws[schemaDiffMediatype]),
		MemberTypes:   schemaDiffIDAliasMap(schemaDiffMembertype, raws[schemaDiffMembertype]),
		Templates:     schemaDiffIDAliasMap(schemaDiffTemplate, raws[schemaDiffTemplate]),
	}

	out := make([]schemaDiffEntity, 0)
	for _, kind := range entities {
		for _, raw := range raws[kind] {
			out = append(out, normalizeSchemaEntity(kind, raw, refs))
		}
	}
	return out, nil
}

func fetchSchemaDiffRawEntities(ctx context.Context, client *api.Client, kind schemaDiffEntityKind) ([]map[string]any, error) {
	switch kind {
	case schemaDiffDoctype:
		return fetchSchemaDiffSchemaTypes(ctx, client, "document-type")
	case schemaDiffMediatype:
		return fetchSchemaDiffSchemaTypes(ctx, client, "media-type")
	case schemaDiffMembertype:
		return fetchSchemaDiffSchemaTypes(ctx, client, "member-type")
	case schemaDiffDatatype:
		return fetchSchemaDiffDatatypes(ctx, client)
	case schemaDiffTemplate:
		return fetchSchemaDiffTemplates(ctx, client)
	case schemaDiffLanguage:
		return fetchSchemaDiffLanguages(ctx, client)
	case schemaDiffDictionary:
		return fetchSchemaDiffDictionary(ctx, client)
	default:
		return nil, fmt.Errorf("unsupported schema diff entity kind %q", kind)
	}
}

func fetchSchemaDiffSchemaTypes(ctx context.Context, client *api.Client, resource string) ([]map[string]any, error) {
	root, err := cmdkit.GetAllPagesWithFallback(ctx, client, cmdkit.AutoPaginateDefaultPageSize, 0, 0,
		cmdkit.GetRequestCandidate{Path: "/tree/" + resource + "/root", Opts: api.RequestOptions{}},
		cmdkit.GetRequestCandidate{Path: "/" + resource + "/root", Opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	items, err := flattenSchemaTypeTree(ctx, client, resource, cmdkit.ResultItems(root), cmdkit.AutoPaginateDefaultPageSize, true, 0)
	if err != nil {
		return nil, err
	}
	return fetchSchemaDiffDetails(ctx, client, "/"+resource+"/%s", items)
}

// fetchSchemaDiffTemplates walks the template tree: templates nest under
// their master template rather than under folders, so every item with
// children is descended regardless of folder-ness.
func fetchSchemaDiffTemplates(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	items, err := collectTemplateTreeItems(ctx, client, "")
	if err != nil {
		return nil, err
	}
	return fetchSchemaDiffDetails(ctx, client, "/template/%s", items)
}

func collectTemplateTreeItems(ctx context.Context, client *api.Client, parentID string) ([]any, error) {
	var candidates []cmdkit.GetRequestCandidate
	if parentID == "" {
		candidates = []cmdkit.GetRequestCandidate{
			{Path: "/tree/template/root", Opts: api.RequestOptions{}},
			{Path: "/template/root", Opts: api.RequestOptions{}},
		}
	} else {
		candidates = []cmdkit.GetRequestCandidate{
			{Path: "/tree/template/children", Opts: api.RequestOptions{Params: map[string]any{"parentId": parentID}}},
		}
	}
	page, err := cmdkit.GetAllPagesWithFallback(ctx, client, cmdkit.AutoPaginateDefaultPageSize, 0, 0, candidates...)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0)
	for _, item := range cmdkit.ResultItems(page) {
		out = append(out, item)
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hasChildren, _ := entry["hasChildren"].(bool)
		id, _ := stringField(entry, "id")
		if !hasChildren || id == "" {
			continue
		}
		children, err := collectTemplateTreeItems(ctx, client, id)
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	return out, nil
}

// fetchSchemaDiffLanguages returns the language list directly: the list
// items are the full language models, so no per-item detail fetch is needed.
func fetchSchemaDiffLanguages(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	page, err := cmdkit.GetAllPagesWithFallback(ctx, client, cmdkit.AutoPaginateDefaultPageSize, 0, 0,
		cmdkit.GetRequestCandidate{Path: "/language", Opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for _, item := range cmdkit.ResultItems(page) {
		if entry, ok := item.(map[string]any); ok {
			out = append(out, entry)
		}
	}
	return out, nil
}

func fetchSchemaDiffDictionary(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	page, err := cmdkit.GetAllPagesWithFallback(ctx, client, cmdkit.AutoPaginateDefaultPageSize, 0, 0,
		cmdkit.GetRequestCandidate{Path: "/dictionary", Opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	overview := cmdkit.ResultItems(page)

	// Only the overview carries each item's parent; /dictionary/{id} returns
	// just id/name/translations. Capture the tree relationship here so a key
	// moved under a different parent is visible to the diff.
	namesByID := map[string]string{}
	parentIDByID := map[string]string{}
	for _, item := range overview {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := stringField(entry, "id")
		if id == "" {
			continue
		}
		if name, ok := stringField(entry, "name"); ok {
			namesByID[id] = name
		}
		if parent, ok := entry["parent"].(map[string]any); ok {
			if parentID, ok := stringField(parent, "id"); ok {
				parentIDByID[id] = parentID
			}
		}
	}

	details, err := fetchSchemaDiffDetails(ctx, client, "/dictionary/%s", overview)
	if err != nil {
		return nil, err
	}
	// Reattach the parent by name (the cross-environment identity for
	// dictionary items) — parent IDs differ across environments by nature.
	for _, detail := range details {
		id, _ := stringField(detail, "id")
		parentID, ok := parentIDByID[id]
		if !ok {
			continue
		}
		if parentName := namesByID[parentID]; parentName != "" {
			detail["parentName"] = parentName
		} else {
			detail["parentName"] = parentID
		}
	}
	return details, nil
}

func fetchSchemaDiffDatatypes(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	result, err := cmdkit.GetAllPagesWithFallback(ctx, client, cmdkit.AutoPaginateDefaultPageSize, 0, 0,
		cmdkit.GetRequestCandidate{Path: dataTypeFilterPath, Opts: api.RequestOptions{}},
		cmdkit.GetRequestCandidate{Path: dataTypeTreeRootPath, Opts: api.RequestOptions{}},
		cmdkit.GetRequestCandidate{Path: dataTypeLegacyCollectionPath, Opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	return fetchSchemaDiffDetails(ctx, client, dataTypeLegacyCollectionPath+"/%s", cmdkit.ResultItems(result))
}

func fetchSchemaDiffDetails(ctx context.Context, client *api.Client, pathFormat string, items []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		itemMap, _ := item.(map[string]any)
		id, _ := stringField(itemMap, "id")
		if id == "" {
			if len(itemMap) > 0 {
				out = append(out, itemMap)
			}
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		detail, err := client.Get(ctx, api.JoinPath(pathFormat, id), api.RequestOptions{})
		if err != nil {
			return nil, err
		}
		detailMap, ok := detail.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema detail %s returned %T, expected object", id, detail)
		}
		out = append(out, detailMap)
	}
	return out, nil
}

func schemaDiffIDAliasMap(kind schemaDiffEntityKind, raws []map[string]any) map[string]string {
	out := map[string]string{}
	for _, raw := range raws {
		id, ok := stringField(raw, "id")
		if !ok {
			continue
		}
		alias := schemaEntityAlias(kind, raw)
		if alias == "" {
			continue
		}
		out[id] = alias
	}
	return out
}

func schemaDiffEntityRequested(entities []schemaDiffEntityKind, target schemaDiffEntityKind) bool {
	if len(entities) == 0 {
		entities = defaultSchemaDiffEntities()
	}
	for _, entity := range entities {
		if entity == target {
			return true
		}
	}
	return false
}

func printSchemaDiffReport(cmd *cobra.Command, deps cmdkit.Dependencies, report schemaDiffReport) error {
	format, err := resolveOutputFormat(deps)
	if err != nil {
		return err
	}
	if format == config.OutputJSON {
		return cmdkit.PrintResult(cmd, deps, report)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), formatSchemaDiffHuman(report))
	return err
}

func formatSchemaDiffHuman(report schemaDiffReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Schema diff %s -> %s\n", report.EnvA, report.EnvB)
	if report.Equal {
		b.WriteString("No differences\n")
		return b.String()
	}
	for _, kind := range report.Entities {
		diff := report.Differences[kind]
		if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Changed) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s\n", kind)
		writeSchemaEntitySummaries(&b, "Added", diff.Added)
		writeSchemaEntitySummaries(&b, "Removed", diff.Removed)
		if len(diff.Changed) > 0 {
			b.WriteString("Changed:\n")
			for _, changed := range diff.Changed {
				fmt.Fprintf(&b, "  - %s", changed.Alias)
				if changed.Name != "" && changed.Name != changed.Alias {
					fmt.Fprintf(&b, " (%s)", changed.Name)
				}
				b.WriteString("\n")
				for _, field := range changed.Fields {
					fmt.Fprintf(&b, "      %s: %v -> %v\n", field.Path, field.Before, field.After)
				}
			}
		}
	}
	return b.String()
}

func writeSchemaEntitySummaries(b *strings.Builder, label string, values []schemaEntitySummary) {
	if len(values) == 0 {
		return
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Alias < values[j].Alias })
	fmt.Fprintf(b, "%s:\n", label)
	for _, value := range values {
		fmt.Fprintf(b, "  - %s", value.Alias)
		if value.Name != "" && value.Name != value.Alias {
			fmt.Fprintf(b, " (%s)", value.Name)
		}
		b.WriteString("\n")
	}
}

func isSchemaDiffFound(err error) bool {
	var diffErr schemaDiffFoundError
	return errors.As(err, &diffErr)
}
