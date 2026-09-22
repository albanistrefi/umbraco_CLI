package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// RegisterSearcher wires the Examine searcher group. It is the read side of
// the indexer group: 'indexer' answers "is the index healthy", 'searcher'
// answers "what does the index actually return for this term".
func RegisterSearcher(root *cobra.Command, deps Dependencies) {
	searcher := &cobra.Command{
		Use:   "searcher",
		Short: "Examine searcher queries",
		Long: `Examine searcher queries.

Task → command:
  Which searchers exist on this instance?              searcher list
  Why is this page missing from search?                searcher query ExternalSearcher --term <text>
  Is the index behind the searcher healthy?            indexer list
  Rebuild the index behind a searcher                  indexer rebuild <index-name> --force --wait
  Only show ids and scores                             searcher query ExternalSearcher --term <text> --fields id,score
  'searcher list' came back empty                      indexer list --fields name,searcherName (try both names)`,
	}
	searcher.AddCommand(searcherList(deps))
	searcher.AddCommand(searcherQuery(deps))
	root.AddCommand(searcher)
}

func searcherList(deps Dependencies) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List Examine searchers (paginated; --skip/--take/--all)",
		Long:  "GET /searcher. Names listed here are the <searcher-name> argument of 'searcher query'. The list only covers searchers registered standalone, so it can be empty on instances that register indexes only: fall back to 'indexer list --fields name,searcherName' and try both names.",
		NArgs: 0,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/searcher", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

// searcherQuery is a collection read rather than a searchCommand: the
// endpoint takes its search text as `term` (not the `query` parameter the
// search archetype sends) and the searcher itself is a path segment, so the
// search builder's NoArgs/`query` contract does not fit. --query is kept as
// an alias of --term for consistency with the other search commands.
func searcherQuery(deps Dependencies) *cobra.Command {
	var term string
	var query string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "query <searcher-name>",
		Short: "Run a term against one Examine searcher (paginated; --skip/--take/--all)",
		Long:  "GET /searcher/{searcherName}/query?term=. Returns the raw Examine hits (id, score, fields) so you can tell \"the document is not in the index\" apart from \"the index has it under different values\". Searcher names come from 'searcher list' or from 'indexer list --fields name,searcherName' — the accepted name is often the index name (ExternalIndex) rather than the searcherName the index reports, so try both when the server answers 404 \"Could not find a valid searcher\". --query is an alias of --term.",
		NArgs: 1,
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: api.JoinPath("/searcher/%s/query", args[0]), opts: api.RequestOptions{Params: withParam(params, "term", searcherTerm(term, query))}},
			}
		},
	})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		return validateSearcherTerm(term, query)
	}
	cmd.Flags().StringVar(&term, "term", "", "Search text sent as the term query parameter (required)")
	cmd.Flags().StringVar(&query, "query", "", "Alias of --term, for consistency with the other search commands")
	return cmd
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
