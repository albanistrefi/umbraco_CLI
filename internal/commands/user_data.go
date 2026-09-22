package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterUserData(root *cobra.Command, deps Dependencies) {
	userData := &cobra.Command{
		Use:   "user-data",
		Short: "Key/value data stored for the authenticated user",
		Long: `Key/value data stored for the authenticated user.

Every endpoint operates on the account the CLI is authenticated as; there is
no way to read or write another user's data.

Task → command:
  What is stored for me?                       user-data list --all
  Narrow to one group or identifier            user-data list --groups <g> --identifiers <i>
  Read one entry by key                        user-data get <key>
  Store a new entry                            user-data create --group <g> --identifier <i> --value <v>
  Replace an entry (all fields required)       user-data update <key> --group <g> --identifier <i> --value <v>
  Remove an entry                              user-data delete <key> --force`,
	}
	userData.AddCommand(userDataList(deps))
	userData.AddCommand(userDataGet(deps))
	userData.AddCommand(userDataCreate(deps))
	userData.AddCommand(userDataUpdate(deps))
	userData.AddCommand(userDataDelete(deps))
	root.AddCommand(userData)
}

func userDataList(deps Dependencies) *cobra.Command {
	var groupsCSV string
	var identifiersCSV string
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: "List the authenticated user's data entries (paginated; --skip/--take/--all)",
		Long:  "GET /user-data. --groups and --identifiers are comma-separated lists sent as repeated query values. --params wins on key collisions.",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			// The spec params map must not be mutated; withParam clones per
			// key and --params keeps precedence on collisions.
			for key, csv := range map[string]string{"groups": groupsCSV, "identifiers": identifiersCSV} {
				values := uniqueCSV(csv)
				if len(values) == 0 {
					continue
				}
				if _, exists := params[key]; exists {
					continue
				}
				params = withParam(params, key, stringsToAny(values))
			}
			return []getRequestCandidate{
				{path: "/user-data", opts: api.RequestOptions{Params: params}},
			}
		},
	})
	cmd.Flags().StringVar(&groupsCSV, "groups", "", "Comma-separated groups to filter by (repeated ?groups=)")
	cmd.Flags().StringVar(&identifiersCSV, "identifiers", "", "Comma-separated identifiers to filter by (repeated ?identifiers=)")
	return cmd
}

func userDataGet(deps Dependencies) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "get <key>",
		Short: "Get one user data entry by key (GUID)",
		Path:  func(args []string) string { return api.JoinPath("/user-data/%s", args[0]) },
	})
}

func userDataDelete(deps Dependencies) *cobra.Command {
	return deleteCommand(deps, deleteSpec{
		Use:   "delete <key>",
		Short: "Permanently delete one user data entry by key (GUID)",
		Path:  func(args []string) string { return api.JoinPath("/user-data/%s", args[0]) },
	})
}

// userDataCreate is written out rather than built with createCommand: the
// create model keys the entry on "key", not the "id" that createCommand
// generates, and the group/identifier/value triple is small enough to pass
// as flags.
func userDataCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var key string
	var group string
	var identifier string
	var value string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user data entry",
		Long:  "POST /user-data. Either pass the full payload via --json, or use the convenience flags (--group, --identifier and --value required). --key is optional; the server assigns one when it is omitted.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := userDataBody(jsonPayload, key, group, identifier, value, false)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/user-data", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().StringVar(&key, "key", "", "Entry key (GUID); the server assigns one when omitted")
	cmd.Flags().StringVar(&group, "group", "", "Entry group")
	cmd.Flags().StringVar(&identifier, "identifier", "", "Entry identifier within the group")
	cmd.Flags().StringVar(&value, "value", "", "Entry value")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// userDataUpdate is written out rather than built with updateCommand: the
// PUT goes to the collection endpoint with the key inside the body, and the
// update model requires every field, so there is nothing to merge against
// (GET /user-data/{id} does not echo the key back).
func userDataUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var group string
	var identifier string
	var value string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <key>",
		Short: "Replace a user data entry (all fields required)",
		Long:  "PUT /user-data with the key in the body. Either pass the full payload via --json, or use the convenience flags (--group, --identifier and --value are all required; the update model has no partial form).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := userDataBody(jsonPayload, args[0], group, identifier, value, true)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), "/user-data", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON")
	cmd.Flags().StringVar(&group, "group", "", "Entry group")
	cmd.Flags().StringVar(&identifier, "identifier", "", "Entry identifier within the group")
	cmd.Flags().StringVar(&value, "value", "", "Entry value")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// userDataBody resolves the --json / convenience-flag pair shared by create
// and update. requireKey marks the update contract, where the key is a
// required body field taken from the positional argument. On update the
// positional key is authoritative: a --json body naming a different entry
// is refused rather than silently rewriting that other entry.
func userDataBody(jsonPayload string, key string, group string, identifier string, value string, requireKey bool) (map[string]any, error) {
	if requireKey {
		if err := requireValue("<key>", key); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(jsonPayload) != "" {
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return nil, err
		}
		if requireKey {
			if bodyKey, ok := body["key"].(string); ok && strings.TrimSpace(bodyKey) != "" && !strings.EqualFold(strings.TrimSpace(bodyKey), strings.TrimSpace(key)) {
				return nil, fmt.Errorf("--json key %s does not match the positional key %s; the argument names the entry to update, so drop the key from --json or correct it", bodyKey, key)
			}
			body["key"] = key
		}
		return body, nil
	}
	// Ordered, so a caller missing several flags always hears about the
	// first one in flag order rather than a random one.
	for _, required := range [][2]string{{"--group", group}, {"--identifier", identifier}, {"--value", value}} {
		if err := requireValue(required[0], required[1]); err != nil {
			return nil, err
		}
	}
	body := map[string]any{
		"group":      group,
		"identifier": identifier,
		"value":      value,
	}
	if strings.TrimSpace(key) != "" {
		body["key"] = key
	}
	return body, nil
}
