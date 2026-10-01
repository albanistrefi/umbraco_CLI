package forms

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

// Forms folders organize the form tree. They are plain {id, name, parentId}
// records on their own routes (/folder/...), separate from the form model.

func formsCreateFolder(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var name string
	var parent string
	var id string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create-folder",
		Short: "Create a Forms folder (optionally inside another folder)",
		Long: "POST /folder. Pass the folder as --json '{\"name\": …, \"parentId\": …}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). " +
			"Put a form inside it with 'forms create --json '{\"name\": …, \"folderId\": \"<folder id>\"}'' or 'forms move <id> --to <folder id>'. After the create the folder is read back, so the result is the persisted record.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := cmdkit.ParseJSONObject(jsonPayload, "--json")
				if err != nil {
					return err
				}
				body = parsed
			}
			fill := map[string]string{"name": name, "parentId": parent, "id": id}
			for key, value := range fill {
				if strings.TrimSpace(value) == "" {
					continue
				}
				if _, set := body[key]; !set {
					body[key] = strings.TrimSpace(value)
				}
			}
			if folderName, _ := body["name"].(string); strings.TrimSpace(folderName) == "" {
				return fmt.Errorf("create-folder requires a folder name: pass --name or a --json payload with \"name\"")
			}
			if parentID, set := body["parentId"]; set && parentID != nil {
				if value, _ := parentID.(string); !cmdkit.IsUUIDLike(value) {
					return fmt.Errorf("parentId must be a Forms folder GUID or null, got %v", parentID)
				}
			}
			if _, set := body["parentId"]; !set {
				body["parentId"] = nil
			}
			folderID, err := cmdkit.EnsurePayloadID(body)
			if err != nil {
				return err
			}
			if !cmdkit.IsUUIDLike(folderID) {
				return fmt.Errorf("id must be a GUID, got %q", folderID)
			}
			ctx := cmd.Context()
			result, err := deps.Client.Post(ctx, "/folder", body, api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			if dryRun {
				return cmdkit.PrintResult(cmd, deps, result)
			}
			created, err := cmdkit.FetchObject(ctx, deps.Client, api.JoinPath("/folder/%s", folderID), formsRequestOpts("", nil))
			if err != nil {
				return fmt.Errorf("the server accepted the folder but reading it back failed: %w", err)
			}
			return cmdkit.PrintResult(cmd, deps, map[string]any{"id": folderID, "name": created["name"], "parentId": created["parentId"], "created": true})
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Folder payload as JSON: {\"id\"?, \"name\", \"parentId\"?}")
	cmd.Flags().StringVar(&name, "name", "", "Folder name (fills name when --json omits it)")
	cmd.Flags().StringVar(&parent, "parent", "", "Parent folder GUID; omit for a root-level folder")
	cmd.Flags().StringVar(&id, "id", "", "Folder GUID to use (generated when omitted)")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

func formsUpdateFolder(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update-folder <id>",
		Short: "Rename a Forms folder",
		Long: "PUT /folder/{id}. The update model carries only the name, e.g. --merge-json '{\"name\": \"New name\"}' or --json '{\"name\": \"New name\"}'. " +
			"Moving a folder is 'forms move-folder'.",
		Path: func(args []string) string { return api.JoinPath("/folder/%s", args[0]) },
		// UpdateFolderModel is {name}; drop what the merge fetch echoes.
		NormalizeMerged: cmdkit.StripFields("id", "parentId", "created"),
		APIPrefix:       formsAPIPrefix,
	})
}

func formsMoveFolder(deps cmdkit.Dependencies) *cobra.Command {
	return formsMoveCommand(deps,
		"move-folder <id>",
		"Move a Forms folder into another folder (or to the root)",
		"PUT /folder/{id}/move. The folder's forms and sub-folders move with it.",
		"/folder/%s/move",
	)
}

func formsDeleteFolder(deps cmdkit.Dependencies) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete-folder <id>",
		Short: "Permanently delete an empty Forms folder",
		Long: "DELETE /folder/{id}. Only empty folders can be deleted: the CLI checks GET /folder/{id}/is-empty first and refuses a folder that still holds forms or sub-folders (and an id that is no folder, which is-empty reports as non-empty), " +
			"because the server answers that case with a bare 500 (a database constraint error) rather than a validation message (verified on Forms 18.1). Move or delete the contents first.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireForceOrDryRun(cmd, "permanently deletes", force, dryRun); err != nil {
				return err
			}
			ctx := cmd.Context()
			empty, err := deps.Client.Get(ctx, api.JoinPath("/folder/%s/is-empty", args[0]), formsRequestOpts("", nil))
			if err != nil {
				return err
			}
			if isEmpty, ok := empty.(bool); ok && !isEmpty {
				// is-empty answers false (not 404) for an id that is no
				// folder at all, so tell a missing folder from a full one.
				folder, probeErr := isFormsFolderID(ctx, deps.Client, args[0])
				if probeErr != nil {
					return probeErr
				}
				if !folder {
					return fmt.Errorf("no Forms folder with id %s; pick a folder with isFolder=true from 'umbraco forms list' or 'umbraco forms children <folderId>'", args[0])
				}
				return fmt.Errorf("forms folder %s is not empty; move or delete its forms and sub-folders first ('umbraco forms children %s')", args[0], args[0])
			}
			result, err := deps.Client.Delete(ctx, api.JoinPath("/folder/%s", args[0]), api.RequestOptions{APIPrefix: formsAPIPrefix, DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}
