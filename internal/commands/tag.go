package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

func RegisterTag(root *cobra.Command, deps cmdkit.Dependencies) {
	tag := &cobra.Command{
		Use:   "tag",
		Short: "Tag reads across tagged content",
		Long: `Tag reads across tagged content.

Task → command:
  Which tags exist at all?                 tag list --all
  Which tags start with a prefix?          tag list --query <text>
  Which tags belong to one tag picker?     tag list --group <tagGroup>
  Which tags exist for one culture?        tag list --culture <isoCode>`,
	}
	tag.AddCommand(tagList(deps))
	root.AddCommand(tag)
}

func tagList(deps cmdkit.Dependencies) *cobra.Command {
	var query string
	var group string
	var culture string
	cmd := cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List tags (paginated; --skip/--take/--all)",
		Long:  "GET /tag. --query matches tag text server-side, --group narrows to one tag group (the tagGroup configured on the tag picker data type), --culture narrows to the tags stored for one language. --params wins on key collisions.",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			// The spec params map must not be mutated; withParam clones per
			// key and --params keeps precedence on collisions.
			for key, value := range map[string]string{"query": query, "tagGroup": group, "culture": culture} {
				if strings.TrimSpace(value) == "" {
					continue
				}
				if _, exists := params[key]; exists {
					continue
				}
				params = cmdkit.WithParam(params, key, value)
			}
			return []cmdkit.GetRequestCandidate{
				{Path: "/tag", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
	cmd.Flags().StringVar(&query, "query", "", "Filter tags by text (maps to ?query=)")
	cmd.Flags().StringVar(&group, "group", "", "Filter by tag group (maps to ?tagGroup=)")
	cmd.Flags().StringVar(&culture, "culture", "", "Filter by culture ISO code (maps to ?culture=)")
	return cmd
}
