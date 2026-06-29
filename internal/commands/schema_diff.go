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
	"umbraco-cli/internal/config"
)

type schemaDiffFoundError struct{}

func (schemaDiffFoundError) Error() string {
	return "schema differences found"
}

func schemaDiffCommand(deps Dependencies) *cobra.Command {
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

	cmd.Flags().StringVar(&entityRaw, "entity", "", "Comma-separated entity kinds to compare: doctype,datatype (default: both)")
	cmd.Flags().StringArrayVar(&include, "include", nil, "Only include matching aliases/names; repeat or comma-separate")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Exclude matching aliases/names; repeat or comma-separate")
	cmd.Flags().BoolVar(&exitZero, "exit-zero", false, "Exit 0 even when schema differences are found")
	return cmd
}

func fetchSchemaDiffEnvironment(ctx context.Context, side string, label string, entities []schemaDiffEntityKind, deps Dependencies) ([]schemaDiffEntity, error) {
	cfg, err := config.LoadWithOptions(config.LoadOptions{Profile: label})
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", side, label, err)
	}
	httpClient := deps.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := api.NewClient(cfg, httpClient, auth.New(cfg, httpClient))

	needsDoctypes := schemaDiffEntityRequested(entities, schemaDiffDoctype)
	needsDatatypes := schemaDiffEntityRequested(entities, schemaDiffDatatype) || needsDoctypes

	var rawDatatypes []map[string]any
	var rawDoctypes []map[string]any
	if needsDatatypes {
		rawDatatypes, err = fetchSchemaDiffRawEntities(ctx, client, schemaDiffDatatype)
		if err != nil {
			return nil, fmt.Errorf("%s %q datatype fetch failed: %w", side, label, err)
		}
	}
	if needsDoctypes {
		rawDoctypes, err = fetchSchemaDiffRawEntities(ctx, client, schemaDiffDoctype)
		if err != nil {
			return nil, fmt.Errorf("%s %q doctype fetch failed: %w", side, label, err)
		}
	}

	refs := schemaDiffReferences{
		DataTypes:     schemaDiffIDAliasMap(schemaDiffDatatype, rawDatatypes),
		DocumentTypes: schemaDiffIDAliasMap(schemaDiffDoctype, rawDoctypes),
	}
	out := make([]schemaDiffEntity, 0, len(rawDatatypes)+len(rawDoctypes))
	if schemaDiffEntityRequested(entities, schemaDiffDoctype) {
		for _, raw := range rawDoctypes {
			out = append(out, normalizeSchemaEntity(schemaDiffDoctype, raw, refs))
		}
	}
	if schemaDiffEntityRequested(entities, schemaDiffDatatype) {
		for _, raw := range rawDatatypes {
			out = append(out, normalizeSchemaEntity(schemaDiffDatatype, raw, refs))
		}
	}
	return out, nil
}

func fetchSchemaDiffRawEntities(ctx context.Context, client *api.Client, kind schemaDiffEntityKind) ([]map[string]any, error) {
	switch kind {
	case schemaDiffDoctype:
		return fetchSchemaDiffDoctypes(ctx, client)
	case schemaDiffDatatype:
		return fetchSchemaDiffDatatypes(ctx, client)
	default:
		return nil, fmt.Errorf("unsupported schema diff entity kind %q", kind)
	}
}

func fetchSchemaDiffDoctypes(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	root, err := getAllPagesWithFallback(ctx, client, autoPaginateDefaultPageSize, 0, 0,
		getRequestCandidate{path: "/tree/document-type/root", opts: api.RequestOptions{}},
		getRequestCandidate{path: "/document-type/root", opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	items, err := flattenDoctypeTree(ctx, client, resultItems(root), autoPaginateDefaultPageSize, true, 0)
	if err != nil {
		return nil, err
	}
	return fetchSchemaDiffDetails(ctx, client, "/document-type/%s", items)
}

func fetchSchemaDiffDatatypes(ctx context.Context, client *api.Client) ([]map[string]any, error) {
	result, err := getAllPagesWithFallback(ctx, client, autoPaginateDefaultPageSize, 0, 0,
		getRequestCandidate{path: dataTypeFilterPath, opts: api.RequestOptions{}},
		getRequestCandidate{path: dataTypeTreeRootPath, opts: api.RequestOptions{}},
		getRequestCandidate{path: dataTypeLegacyCollectionPath, opts: api.RequestOptions{}},
	)
	if err != nil {
		return nil, err
	}
	return fetchSchemaDiffDetails(ctx, client, dataTypeLegacyCollectionPath+"/%s", resultItems(result))
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

func printSchemaDiffReport(cmd *cobra.Command, deps Dependencies, report schemaDiffReport) error {
	format, err := resolveOutputFormat(deps)
	if err != nil {
		return err
	}
	if format == config.OutputJSON {
		return printResult(cmd, deps, report)
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
