package commands

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// RegisterSearcher wires the Examine searcher group. It is the read side of
// the indexer group: 'indexer' answers "is the index healthy", 'searcher'
// answers "what does the index actually return for this term".
func RegisterSearcher(root *cobra.Command, deps cmdkit.Dependencies) {
	searcher := &cobra.Command{
		Use:   "searcher",
		Short: "Examine searcher queries",
		Long: `Examine searcher queries.

'searcher query' takes the INDEX name (ExternalIndex, InternalIndex, …) that 'indexer list' reports in its name field.
The searcherName 'indexer list' also reports (ExternalSearcher) answers 404 on Umbraco 18.2, and 'searcher list'
returns an empty list there: that is how the Management API behaves, not a sign that search is broken.

Task → command:
  Which names can I query?                             indexer list --fields name
  Why is this page missing from search?                searcher query ExternalIndex --term <text>
  Is the index healthy?                                indexer list
  Rebuild an index                                     indexer rebuild <index-name> --force --wait
  Only show ids and scores                             searcher query ExternalIndex --term <text> --fields id,score
  'searcher list' came back empty                      expected on 18.2; use indexer list --fields name for the names`,
	}
	searcher.AddCommand(searcherList(deps))
	searcher.AddCommand(searcherQuery(deps))
	root.AddCommand(searcher)
}

func searcherList(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List Examine searchers (paginated; --skip/--take/--all)",
		Long:  "GET /searcher. Lists the searchers the server registers on their own. On Umbraco 18.2 this returns an empty list: that is how the Management API behaves, not a sign that search is broken. 'searcher query' takes the index name, which 'indexer list --fields name' lists (ExternalIndex, InternalIndex).",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/searcher", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

// searcherQuery is a collection read rather than a cmdkit.SearchCommand: the
// endpoint takes its search text as `term` (not the `query` parameter the
// search archetype sends) and the searcher itself is a path segment, so the
// search builder's NoArgs/`query` contract does not fit. --query is kept as
// an alias of --term for consistency with the other search commands.
func searcherQuery(deps cmdkit.Dependencies) *cobra.Command {
	var term string
	var query string
	cmd := cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "query <index-name>",
		Short: "Run a term against one Examine index (paginated; --skip/--take/--all)",
		Long:  "GET /searcher/{searcherName}/query?term=. Returns the raw Examine hits (id, score, fields) so you can tell \"the document is not in the index\" apart from \"the index has it under different values\". Pass the index name from 'indexer list --fields name', e.g. ExternalIndex or InternalIndex: on Umbraco 18.2 the searcherName 'indexer list' reports (ExternalSearcher) answers 404 \"Could not find a valid searcher\", and 'searcher list' is empty. --query is an alias of --term.",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			// Raw --params wins on collisions (CONVENTIONS.md), so the flag
			// only fills term when --params did not supply one. The clone
			// keeps the caller's params map untouched.
			searchParams := cmdkit.MergeParams(maps.Clone(params), map[string]any{"term": searcherTerm(term, query)})
			return []cmdkit.GetRequestCandidate{
				{Path: api.JoinPath("/searcher/%s/query", args[0]), Opts: api.RequestOptions{Params: searchParams}},
			}
		},
	})
	cmd.Example = `  umbraco indexer list --fields name
  umbraco searcher query ExternalIndex --term <text>
  umbraco searcher query ExternalIndex --term <text> --fields id,score`
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return searcherNameHint(run(cmd, args), args[0])
	}
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		paramsTerm, err := searcherParamsTerm(cmd)
		if err != nil {
			return err
		}
		// A term supplied through --params is a term: requiring the flag as
		// well would reject a request the command can serve.
		if paramsTerm != "" {
			return nil
		}
		return validateSearcherTerm(term, query)
	}
	cmd.Flags().StringVar(&term, "term", "", "Search text sent as the term query parameter (required)")
	cmd.Flags().StringVar(&query, "query", "", "Alias of --term, for consistency with the other search commands")
	return cmd
}

// searcherNameHint points a 404 for a name ending in "Searcher" (the
// searcherName 'indexer list' reports) at the matching index name, which is
// what the endpoint accepts on Umbraco 18.2. It replaces the client's
// generic route-not-found hint, which points the wrong way here. The
// *api.APIError (and so exit code 4) is kept.
func searcherNameHint(err error, name string) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return err
	}
	const suffix = "searcher"
	if len(name) <= len(suffix) || !strings.EqualFold(name[len(name)-len(suffix):], suffix) {
		return err
	}
	apiErr.Hint = fmt.Sprintf("searcher query takes the index name, not the searcherName: try 'umbraco searcher query %sIndex --term <text>' ('umbraco indexer list --fields name' lists the index names)", name[:len(name)-len(suffix)])
	return err
}

// searcherParamsTerm reads the term a caller passed through raw --params, so
// the required-term check does not reject a request that already has one.
func searcherParamsTerm(cmd *cobra.Command) (string, error) {
	flag := cmd.Flags().Lookup("params")
	if flag == nil {
		return "", nil
	}
	params, err := cmdkit.ParseParams(flag.Value.String())
	if err != nil {
		return "", err
	}
	value, _ := params["term"].(string)
	return strings.TrimSpace(value), nil
}

func searcherTerm(term, query string) string {
	if strings.TrimSpace(term) != "" {
		return term
	}
	return query
}

func validateSearcherTerm(term, query string) error {
	trimmedTerm := strings.TrimSpace(term)
	trimmedQuery := strings.TrimSpace(query)
	if trimmedTerm == "" && trimmedQuery == "" {
		return fmt.Errorf("searcher query requires --term <text> (or its alias --query)")
	}
	// Silently preferring one over the other would search for something the
	// caller did not ask for, so conflicting values are an error.
	if trimmedTerm != "" && trimmedQuery != "" && trimmedTerm != trimmedQuery {
		return fmt.Errorf("--term and --query are aliases and were given different values (%q vs %q); pass only one", term, query)
	}
	return nil
}
