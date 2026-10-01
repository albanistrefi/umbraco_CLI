package forms

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// Form authoring: create, change, copy, move, and delete form definitions.
// What an API user may do is decided by its Forms permissions (manage
// forms, manage workflows, ...), not by the CLI; a refused write comes back
// as the server's 403.

func formsCreate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CreateCommand(deps, cmdkit.CreateSpec{
		Use:   "create",
		Short: "Create a form, starting from the server's form scaffold",
		Long: "POST /form. The full form model has some thirty required fields, so the CLI fetches GET /form/scaffold (the same starting point the backoffice uses: a fresh id, the default page/field layout, and the install's default workflows) and deep-merges --json on top; only what differs from the scaffold needs naming, and \"name\" is required. " +
			"Put the form in a folder with {\"folderId\": \"<folder id>\"} (folders from 'forms list'/'forms children', isFolder=true; omit for the root). " +
			"The scaffold's default workflows (Umbraco:Forms:Options:DefaultWorkflows, e.g. a notification email) are kept unless --json replaces them, e.g. {\"formWorkflows\": {\"onSubmit\": [], \"onApprove\": [], \"onReject\": []}}; pages and other arrays are replaced wholesale, not merged. " +
			"The result carries the new form's id; read it back with 'forms get <id>'.",
		Path:         "/form",
		TemplateKey:  "forms.create",
		PayloadUsage: "Form fields as JSON, deep-merged onto GET /form/scaffold; must name \"name\"",
		Base: func(ctx context.Context) (map[string]any, error) {
			return cmdkit.FetchObject(ctx, deps.Client, "/form/scaffold", formsRequestOpts("", nil))
		},
		Flags: func(cmd *cobra.Command) func(map[string]any) error {
			return func(body map[string]any) error {
				if name, _ := body["name"].(string); strings.TrimSpace(name) == "" {
					return fmt.Errorf("forms create requires a form name: pass --json '{\"name\": \"<form name>\"}'")
				}
				// The scaffold mirrors its id into "unique"; a caller-chosen
				// id must not leave the scaffold's stale one behind.
				if id, ok := body["id"].(string); ok && strings.TrimSpace(id) != "" {
					if _, set := body["unique"]; set {
						body["unique"] = id
					}
				}
				return nil
			}
		},
		ResultKeys: []string{"folderId"},
		APIPrefix:  formsAPIPrefix,
	})
}

// bindFormsBodyID makes an update body carry the id of the resource being
// updated. The Forms API answers 200 to a PUT /form/{id} whose body has no
// id, leaves that form untouched and instead creates a new form at the
// root with a fresh id (verified on Forms 18.1), so the id is filled in
// from the argument; a body naming a different id is refused rather than
// sent, since the path and the body would then disagree about the target.
func bindFormsBodyID(args []string, body map[string]any) error {
	id := args[0]
	if existing, ok := body["id"]; ok && existing != nil {
		value, _ := existing.(string)
		if !strings.EqualFold(strings.TrimSpace(value), id) {
			return fmt.Errorf("the payload's id %v does not match the id argument %s; drop the id from the payload or pass the matching one", existing, id)
		}
	}
	body["id"] = id
	return nil
}

func formsUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update <id>",
		Short: "Update a form definition (fields, pages, workflows, settings)",
		Long: "PUT /form/{id}. --merge-json fetches the form and deep-merges the patch, so unmentioned settings survive — the safe default (e.g. --merge-json '{\"name\": \"New name\"}'). " +
			"--json is a full replacement: every field it leaves out is reset to its default, including folderId (the form moves to the root) and pages (the form loses its fields) — start from 'forms get <id>' output when using it. " +
			"Arrays such as pages and formWorkflows.onSubmit are replaced wholesale by a patch, not merged entry by entry. " +
			"The body's id is filled in from the argument — a PUT whose body has no id leaves the form untouched and creates a new one at the root instead — and a mismatching one is refused. " +
			"Workflows live on the form: edit formWorkflows here, or copy them from another form with 'forms copy-workflows'.",
		Path:      func(args []string) string { return api.JoinPath("/form/%s", args[0]) },
		BindArgs:  bindFormsBodyID,
		APIPrefix: formsAPIPrefix,
	})
}

func formsDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:       "delete <id>",
		Short:     "Permanently delete a form together with its stored records",
		Path:      func(args []string) string { return api.JoinPath("/form/%s", args[0]) },
		APIPrefix: formsAPIPrefix,
	})
}

func formsCopy(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var name string
	var to string
	var copyWorkflows bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "copy <id>",
		Short: "Copy a form (optionally with its workflows, into another folder)",
		Long: "POST /form/{id}/copy. --name names the copy (the server otherwise appends \" (1)\" to the source name), --to puts it in a folder (default: the source form's folder), and --copy-workflows carries the source's workflows over. " +
			"--json sends the raw body instead: {\"newName\", \"copyWorkflows\", \"copyToFolderId\"}. The result carries the new form's id.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				if cmd.Flags().Changed("name") || cmd.Flags().Changed("to") || cmd.Flags().Changed("copy-workflows") {
					return fmt.Errorf("--json cannot be combined with --name, --to or --copy-workflows")
				}
				parsed, err := cmdkit.ParsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				body = map[string]any{"copyWorkflows": copyWorkflows}
				if strings.TrimSpace(name) != "" {
					body["newName"] = strings.TrimSpace(name)
				}
				if strings.TrimSpace(to) != "" {
					if !cmdkit.IsUUIDLike(to) {
						return fmt.Errorf("--to must be a Forms folder GUID, got %q", to)
					}
					body["copyToFolderId"] = strings.TrimSpace(to)
				}
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/form/%s/copy", args[0]), body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				return cmdkit.PrintResult(cmd, deps, result)
			}
			out := map[string]any{"copied": true, "sourceId": args[0]}
			if created, ok := result.(map[string]any); ok {
				for key, value := range created {
					out[key] = value
				}
			}
			if newName, ok := body["newName"]; ok {
				out["name"] = newName
			}
			return cmdkit.PrintResult(cmd, deps, out)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Raw copy payload as JSON: {\"newName\"?, \"copyWorkflows\", \"copyToFolderId\"?}")
	cmd.Flags().StringVar(&name, "name", "", "Name for the copy (default: the source name with \" (1)\" appended)")
	cmd.Flags().StringVar(&to, "to", "", "Folder GUID to put the copy in (default: the source form's folder)")
	cmd.Flags().BoolVar(&copyWorkflows, "copy-workflows", false, "Copy the source form's workflows too")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// formsMoveCommand builds the folder-reparenting moves shared by forms and
// folders: PUT <path>/move with {"parentId": <folder id or null>}.
func formsMoveCommand(deps cmdkit.Dependencies, use string, short string, long string, pathFormat string) *cobra.Command {
	var jsonPayload string
	var to string
	var toRoot bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := formsMoveBody(jsonPayload, to, toRoot)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), api.JoinPath(pathFormat, args[0]), body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "moved", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Raw move payload as JSON: {\"parentId\": \"<folder id>\" | null}")
	cmd.Flags().StringVar(&to, "to", "", "Target folder GUID")
	cmd.Flags().BoolVar(&toRoot, "to-root", false, "Move to the root of the Forms tree")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// formsMoveBody resolves the exactly-one-of --json / --to / --to-root
// choice into the {"parentId"} move body.
func formsMoveBody(jsonPayload string, to string, toRoot bool) (map[string]any, error) {
	given := 0
	for _, set := range []bool{strings.TrimSpace(jsonPayload) != "", strings.TrimSpace(to) != "", toRoot} {
		if set {
			given++
		}
	}
	if given != 1 {
		return nil, fmt.Errorf("move requires exactly one of --to <folder id>, --to-root or --json")
	}
	switch {
	case strings.TrimSpace(jsonPayload) != "":
		body, err := cmdkit.ParsePayload(jsonPayload)
		if err != nil {
			return nil, err
		}
		// A missing parentId deserialises like null, which means "move to
		// the root"; require it to be stated so --json '{}' cannot do that.
		parentID, present := body["parentId"]
		if !present {
			return nil, fmt.Errorf("--json must set \"parentId\": a Forms folder GUID, or null to move to the root (or use --to / --to-root)")
		}
		if parentID != nil {
			text, ok := parentID.(string)
			if !ok || !cmdkit.IsUUIDLike(text) {
				return nil, fmt.Errorf("parentId must be a Forms folder GUID or null, got %v", parentID)
			}
		}
		return body, nil
	case toRoot:
		return map[string]any{"parentId": nil}, nil
	default:
		if !cmdkit.IsUUIDLike(to) {
			return nil, fmt.Errorf("--to must be a Forms folder GUID, got %q", to)
		}
		return map[string]any{"parentId": strings.TrimSpace(to)}, nil
	}
}

func formsMove(deps cmdkit.Dependencies) *cobra.Command {
	return formsMoveCommand(deps,
		"move <id>",
		"Move a form into another folder (or to the root)",
		"PUT /form/{id}/move. --to takes a folder id (isFolder=true in 'forms list'/'forms children'); --to-root moves the form out of every folder.",
		"/form/%s/move",
	)
}

func formsCopyWorkflows(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var workflowIDs string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "copy-workflows <sourceFormId>",
		Short: "Copy workflows from one form onto another",
		Long: "POST /form/{sourceFormId}/copy-workflows. --workflow-ids names the source form's workflows to copy (ids from 'forms get <sourceFormId> --fields formWorkflows'); --to is the destination form. " +
			"The copies are added to the destination's existing workflows. --json sends the raw body instead: {\"destinationId\", \"workflowIds\": [...]}.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				if strings.TrimSpace(to) != "" || strings.TrimSpace(workflowIDs) != "" {
					return fmt.Errorf("--json cannot be combined with --to or --workflow-ids")
				}
				parsed, err := cmdkit.ParsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				if !cmdkit.IsUUIDLike(to) {
					return fmt.Errorf("copy-workflows requires --to <destination form GUID>")
				}
				ids := cmdkit.UniqueCSV(workflowIDs)
				if len(ids) == 0 {
					return fmt.Errorf("copy-workflows requires --workflow-ids <comma-separated workflow GUIDs>")
				}
				for _, id := range ids {
					if !cmdkit.IsUUIDLike(id) {
						return fmt.Errorf("--workflow-ids must be workflow GUIDs, got %q", id)
					}
				}
				body = map[string]any{"destinationId": strings.TrimSpace(to), "workflowIds": cmdkit.StringsToAny(ids)}
			}
			result, err := deps.Client.Post(cmd.Context(), api.JoinPath("/form/%s/copy-workflows", args[0]), body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "copied", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Raw payload as JSON: {\"destinationId\", \"workflowIds\": [...]}")
	cmd.Flags().StringVar(&to, "to", "", "Destination form GUID")
	cmd.Flags().StringVar(&workflowIDs, "workflow-ids", "", "Comma-separated GUIDs of the source form's workflows to copy")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}
