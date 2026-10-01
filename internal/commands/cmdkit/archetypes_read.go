package cmdkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// GetSpec configures GetCommand.
type GetSpec struct {
	Use   string
	Short string
	Long  string
	// Path maps the positional args to the resource path.
	Path func(args []string) string
	// APIPrefix overrides the default core Management API mount.
	APIPrefix string
}

// GetCommand builds a get-by-ID read: --fields wiring plus client-side
// projection for endpoints that ignore the fields hint server-side.
func GetCommand(deps Dependencies, spec GetSpec) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), spec.Path(args), api.RequestOptions{Fields: fields, APIPrefix: spec.APIPrefix})
			if err != nil {
				return err
			}
			return PrintResult(cmd, deps, ApplyFieldsProjection(result, fields))
		},
	}
	AddFieldsFlag(cmd, &fields)
	return cmd
}

// CollectionSpec configures CollectionCommand.
type CollectionSpec struct {
	Use   string
	Short string
	Long  string
	NArgs int
	// DocumentOutputTrim enables document-specific --summary/--no-empty/--full
	// output shaping in addition to --fields.
	DocumentOutputTrim bool
	// Args overrides the default cobra.ExactArgs(NArgs) validation for
	// commands with optional positional arguments.
	Args cobra.PositionalArgs
	// Endpoints maps the positional args and resolved query params to the
	// candidate endpoints in fallback order. Params must not be mutated;
	// candidates that need extra keys clone via WithParam.
	Endpoints func(args []string, params map[string]any) []GetRequestCandidate
	// Enrich, when non-nil, post-processes the fetched result before
	// projection and triage (e.g. resolving referenced entity names).
	Enrich func(ctx context.Context, result any) (any, error)
}

// CollectionCommand builds a paginated collection read (root/children/list):
// --fields/--params/--skip/--take/--all/triage wiring, endpoint fallback,
// auto-pagination, and projection.
func CollectionCommand(deps Dependencies, spec CollectionSpec) *cobra.Command {
	var fields string
	var trim OutputTrimOptions
	var paramsRaw string
	var skip, take int
	var all bool
	var triage ReadTriageOptions
	positionalArgs := spec.Args
	if positionalArgs == nil {
		positionalArgs = cobra.ExactArgs(spec.NArgs)
	}
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  positionalArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spec.DocumentOutputTrim {
				if err := ValidateDocumentOutputTrim(trim); err != nil {
					return err
				}
			}
			params, err := ParseParams(paramsRaw)
			if err != nil {
				return err
			}
			params = ApplyPaginationParams(params, skip, take)
			candidates := spec.Endpoints(args, params)
			if spec.DocumentOutputTrim {
				fields = trim.Fields
			}
			for i := range candidates {
				candidates[i].Opts.Fields = fields
			}

			ctx := cmd.Context()
			var result any
			if all {
				result, err = GetAllPagesWithFallback(ctx, deps.Client, take, skip, triage.FirstN, candidates...)
			} else {
				result, err = GetWithFallback(ctx, deps.Client, candidates...)
			}
			if err != nil {
				return err
			}
			if spec.Enrich != nil {
				result, err = spec.Enrich(ctx, result)
				if err != nil {
					return err
				}
			}
			if spec.DocumentOutputTrim {
				result, err = ApplyDocumentOutputTrim(result, trim, cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				return PrintResult(cmd, deps, ApplyReadTriage(result, triage))
			}
			return PrintResult(cmd, deps, ApplyReadTriage(ApplyFieldsProjection(result, fields), triage))
		},
	}
	if spec.DocumentOutputTrim {
		AddDocumentOutputTrimFlags(cmd, &trim)
	} else {
		AddFieldsFlag(cmd, &fields)
	}
	cmd.Flags().StringVar(&paramsRaw, "params", "", "Query parameters as JSON")
	AddPaginationFlags(cmd, &skip, &take)
	AddAutoPaginationFlag(cmd, &all)
	AddReadTriageFlags(cmd, &triage)
	return cmd
}

// ParamFlag declares a string convenience flag that maps onto a query
// parameter for search commands.
type ParamFlag struct {
	Flag  string
	Param string
	Usage string
}

// SearchSpec configures SearchCommand.
type SearchSpec struct {
	Use   string
	Short string
	Long  string
	// DocumentOutputTrim enables document-specific --fields/--summary output
	// shaping for document search results.
	DocumentOutputTrim bool
	// Flags beyond the always-present --query, e.g. --under → parentId.
	Extra []ParamFlag
	// Endpoints maps the resolved query params to candidates in fallback order.
	Endpoints func(params map[string]any) []GetRequestCandidate
	// Enrich, when non-nil, post-processes the fetched result before output
	// (e.g. adding aliases the item search does not return).
	Enrich func(ctx context.Context, result any) (any, error)
}

// SearchCommand builds a search read with the uniform parameter contract:
// convenience flags (--query, --skip, --take, spec extras) merge into
// --params, with --params winning on key collisions.
func SearchCommand(deps Dependencies, spec SearchSpec) *cobra.Command {
	var paramsRaw string
	var query string
	var skip, take int
	var trim OutputTrimOptions
	extraValues := make([]string, len(spec.Extra))

	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spec.DocumentOutputTrim {
				if err := ValidateDocumentOutputTrim(trim); err != nil {
					return err
				}
			}
			params, err := ParseParams(paramsRaw)
			if err != nil {
				return err
			}

			flagValues := map[string]any{}
			if strings.TrimSpace(query) != "" {
				flagValues["query"] = query
			}
			for i, extra := range spec.Extra {
				if strings.TrimSpace(extraValues[i]) != "" {
					flagValues[extra.Param] = extraValues[i]
				}
			}
			if skip >= 0 {
				flagValues["skip"] = skip
			}
			if take >= 0 {
				flagValues["take"] = take
			}

			params = MergeParams(params, flagValues)
			if len(params) == 0 {
				return fmt.Errorf("%s requires either --params or --query", cmd.CommandPath())
			}

			result, err := GetWithFallback(cmd.Context(), deps.Client, spec.Endpoints(params)...)
			if err != nil {
				return err
			}
			if spec.Enrich != nil {
				if result, err = spec.Enrich(cmd.Context(), result); err != nil {
					return err
				}
			}
			if spec.DocumentOutputTrim {
				result, err = ApplyDocumentOutputTrim(result, trim, cmd.ErrOrStderr())
				if err != nil {
					return err
				}
			}
			return PrintResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	if spec.DocumentOutputTrim {
		AddDocumentOutputTrimFlags(cmd, &trim)
	}
	for i, extra := range spec.Extra {
		cmd.Flags().StringVar(&extraValues[i], extra.Flag, "", extra.Usage)
	}
	AddPaginationFlags(cmd, &skip, &take)
	return cmd
}

// ReferencesSpec configures ReferencesCommand.
type ReferencesSpec struct {
	Use   string
	Short string
	Long  string
	Path  func(args []string) string
}

// ReferencesCommand builds the paginated 'what references this' reads
// (referenced-by / referenced-descendants) shared by document and media.
// The endpoints return the standard {items, total} envelope, so pagination
// and triage compose the same way they do on children/root.
func ReferencesCommand(deps Dependencies, spec ReferencesSpec) *cobra.Command {
	return CollectionCommand(deps, CollectionSpec{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []GetRequestCandidate {
			return []GetRequestCandidate{
				{Path: spec.Path(args), Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

// AreReferencedCommand builds the bulk reference check shared by document
// and media: GET /<resource>/are-referenced?id=...&id=...
func AreReferencedCommand(deps Dependencies, resource string) *cobra.Command {
	var idsCSV string
	cmd := &cobra.Command{
		Use:   "are-referenced",
		Short: fmt.Sprintf("Bulk check: which of these %s IDs are referenced by something", resource),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := UniqueCSV(idsCSV)
			if len(ids) == 0 {
				return fmt.Errorf("%s are-referenced requires --ids <comma-separated guids>", resource)
			}
			// The endpoint is paginated with a default take of 20; ask for
			// as many rows as IDs supplied so no referenced ID is silently
			// dropped and misread as unreferenced.
			result, err := deps.Client.Get(cmd.Context(), "/"+resource+"/are-referenced", api.RequestOptions{Params: map[string]any{"id": StringsToAny(ids), "skip": 0, "take": len(ids)}})
			if err != nil {
				return err
			}
			return PrintResult(cmd, deps, result)
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", fmt.Sprintf("Comma-separated %s GUIDs to check (required)", resource))
	return cmd
}
