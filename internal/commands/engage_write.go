package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Engage saves configuration entities through one upsert route per entity:
// the back office's create and update both POST the whole entity, and the
// server tells them apart by the numeric `id` (0 = new). The CLI keeps the
// two apart on purpose so neither can do the other's job by accident:
//
//   - create always sends `id: 0` and a GUID it generates when none is
//     given, and rejects a non-zero `id` in --json;
//   - update always fetches the current entity first, so it fails on a GUID
//     that does not exist instead of silently creating a new entity, and
//     pins the server's numeric `id` and the GUID onto the body.
//
// Engage's routes and body shapes come from its OpenAPI document (Engage
// 18.1.0) and the back-office client that calls them.

type engageWriteSpec struct {
	// SavePath is the upsert route both create and update POST to.
	SavePath string
	// DeletePath, when non-empty, adds a force-gated delete; DeleteParam
	// names its query parameter.
	DeletePath  string
	DeleteParam string
	// BodyIDField is the GUID field in the save body (`unique`, or `key` on
	// traffic filters). It can differ from the read side's IDField: goals
	// list a `key` but save and detail models call it `unique`.
	BodyIDField string
	// EmptyPath, when non-empty, is the server's blank create template that
	// --print-template prints; otherwise --print-template prints Scaffold.
	EmptyPath string
	// Scaffold returns the create defaults the caller's --json is merged
	// onto, mirroring the back office's create scaffolds.
	Scaffold func() map[string]any
	// Nested maps array fields to the GUID field their entries need; an
	// entry without one gets a fresh GUID, as the back office does.
	Nested map[string]string
	// Normalize adjusts or rejects the final body (create and update).
	Normalize func(map[string]any) error
	// Notes is appended to the create and update help.
	Notes string
}

// engageEntityWrites builds create/update/(delete) for one entity.
func engageEntityWrites(deps Dependencies, entity engageEntity) []*cobra.Command {
	spec := entity.Write
	commands := []*cobra.Command{engageCreate(deps, entity), engageUpdate(deps, entity)}
	if spec.DeletePath != "" {
		commands = append(commands, engageDelete(deps, engageDeleteSpec{
			Use:         "delete <" + entity.IDField + ">",
			Short:       fmt.Sprintf("Permanently delete a %s by its GUID `%s`", entity.Noun, entity.IDField),
			Long:        fmt.Sprintf("DELETE %s?%s=<%s>. Requires --force (or --dry-run to rehearse).", spec.DeletePath, spec.DeleteParam, entity.IDField),
			Path:        spec.DeletePath,
			Param:       spec.DeleteParam,
			IDKind:      engageGUID,
			IDField:     entity.IDField,
			ListCommand: "umbraco engage " + entity.Group + " list",
			Consequence: "permanently deletes the " + entity.Noun,
		}))
	}
	return commands
}

func engageCreate(deps Dependencies, entity engageEntity) *cobra.Command {
	spec := entity.Write
	var jsonPayload string
	var dryRun, printTemplate bool
	template := "a built-in scaffold mirroring the back office's"
	if spec.EmptyPath != "" {
		template = "GET " + spec.EmptyPath + ", the server's blank template"
	}
	cmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s (POST %s)", entity.Noun, spec.SavePath),
		Long: fmt.Sprintf("POST %s with `id` 0. --json is merged onto the create defaults; `%s` is generated when omitted. "+
			"A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. "+
			"--print-template prints %s.%s", spec.SavePath, spec.BodyIDField, template, spec.Notes),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				if spec.EmptyPath == "" {
					return printResult(cmd, deps, spec.Scaffold())
				}
				result, err := engageGet(cmd, deps, spec.EmptyPath, nil)
				if err != nil {
					return err
				}
				return printResult(cmd, deps, result)
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			userBody, err := parsePayload(jsonPayload)
			if err != nil {
				return err
			}
			if id, ok := userBody["id"]; ok && !isZeroEngageID(id) {
				return fmt.Errorf("engage %s create sends `id` 0; --json carries `id` %v, which would update that %s. Use 'umbraco engage %s update <%s>' instead", entity.Group, id, entity.Noun, entity.Group, entity.IDField)
			}
			body := mergeAliasPayload(spec.Scaffold(), userBody)
			body["id"] = 0
			if err := ensureEngageGUID(body, spec.BodyIDField); err != nil {
				return err
			}
			if err := finishEngageBody(body, spec); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), spec.SavePath, body, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			return printEngageSave(cmd, deps, "created", result, body, spec.BodyIDField, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON (merged onto the create defaults)")
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print a JSON skeleton; edit it and pass it to --json")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func engageUpdate(deps Dependencies, entity engageEntity) *cobra.Command {
	spec := entity.Write
	var jsonPayload, mergeJSON string
	var dryRun bool
	listCommand := "umbraco engage " + entity.Group + " list"
	cmd := &cobra.Command{
		Use:   "update <" + entity.IDField + ">",
		Short: fmt.Sprintf("Update a %s by its GUID `%s` (POST %s)", entity.Noun, entity.IDField, spec.SavePath),
		Long: fmt.Sprintf("Fetches GET %s?%s=<%s> (also under --dry-run, so an unknown GUID fails instead of creating a new %s), then POSTs %s with the entity's numeric `id` and `%s` pinned. "+
			"Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry).%s",
			entity.GetPath, entity.IDParam, entity.IDField, entity.Noun, spec.SavePath, spec.BodyIDField, spec.Notes),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := "engage " + entity.Group + " update"
			if err := validateEngageID(command, args[0], engageGUID, entity.IDField, listCommand); err != nil {
				return err
			}
			guid := strings.TrimSpace(args[0])
			hasJSON := strings.TrimSpace(jsonPayload) != ""
			if hasJSON == (strings.TrimSpace(mergeJSON) != "") {
				return fmt.Errorf("update requires exactly one of --json (full replacement) or --merge-json (fetch and merge)")
			}
			var input map[string]any
			var err error
			if hasJSON {
				input, err = parsePayload(jsonPayload)
			} else {
				input, err = parseJSONObject(mergeJSON, "--merge-json")
			}
			if err != nil {
				return err
			}
			current, err := engageFetchObject(cmd.Context(), deps, entity.GetPath, map[string]any{entity.IDParam: guid})
			if err != nil {
				return err
			}
			body := input
			if !hasJSON {
				body = mergeAliasPayload(current, input)
			}
			if err := pinEngageIdentity(command, body, current, spec.BodyIDField, guid); err != nil {
				return err
			}
			if err := finishEngageBody(body, spec); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), spec.SavePath, body, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			return printEngageSave(cmd, deps, "updated", result, body, spec.BodyIDField, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full replacement payload as JSON (fields not mentioned are reset by the server)")
	cmd.Flags().StringVar(&mergeJSON, "merge-json", "", "Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved)")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

type engageDeleteSpec struct {
	Use         string
	Short       string
	Long        string
	Path        string
	Param       string
	IDKind      engageIDKind
	IDField     string
	ListCommand string
	Consequence string
}

// engageDelete is deleteCommand for Engage's query-parameter ids, with the
// id kind validated before the force gate so a wrong id never needs --force
// to be noticed.
func engageDelete(deps Dependencies, spec engageDeleteSpec) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateEngageID("engage "+cmd.Parent().Name()+" "+cmd.Name(), args[0], spec.IDKind, spec.IDField, spec.ListCommand); err != nil {
				return err
			}
			if err := requireForceOrDryRun(cmd, spec.Consequence, force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), spec.Path, engageWriteOpts(map[string]any{spec.Param: strings.TrimSpace(args[0])}, dryRun))
			if err != nil {
				return engageError(err)
			}
			if err := printMutationResult(cmd, deps, "deleted", result, dryRun); err != nil {
				return err
			}
			return engageValidationFailure(cmd, result)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func engageWriteOpts(params map[string]any, dryRun bool) api.RequestOptions {
	opts := engageOpts(params)
	opts.DryRun = dryRun
	return opts
}

func engageFetchObject(ctx context.Context, deps Dependencies, path string, params map[string]any) (map[string]any, error) {
	result, err := deps.Client.Get(ctx, path, engageOpts(params))
	if err != nil {
		return nil, engageError(err)
	}
	return objectFromResult("GET "+path, result)
}

// isZeroEngageID reports whether a body `id` means "new": 0, "0", or null.
func isZeroEngageID(value any) bool {
	switch id := value.(type) {
	case nil:
		return true
	case float64:
		return id == 0
	case int:
		return id == 0
	case string:
		return strings.TrimSpace(id) == "" || strings.TrimSpace(id) == "0"
	}
	return false
}

// ensureEngageGUID keeps a supplied GUID, generates one for an absent, null
// or empty field, and rejects anything else: a number or object in the GUID
// field is malformed input, not a request for a fresh identity.
func ensureEngageGUID(body map[string]any, field string) error {
	if value, present := body[field]; present && value != nil {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("`%s` must be a GUID string, got %s", field, jsonShapeName(value))
		}
		if strings.TrimSpace(text) != "" {
			if !isUUIDLike(text) {
				return fmt.Errorf("`%s` must be a GUID, got %q", field, text)
			}
			return nil
		}
	}
	guid, err := newUUIDv4()
	if err != nil {
		return fmt.Errorf("failed to generate `%s`: %w", field, err)
	}
	body[field] = guid
	return nil
}

// pinEngageIdentity makes the update body address the fetched entity: the
// numeric `id` comes from the server and the GUID from the argument. A body
// naming a different entity is rejected rather than silently retargeted.
func pinEngageIdentity(command string, body map[string]any, current map[string]any, field string, guid string) error {
	serverID, ok := current["id"]
	if !ok || isZeroEngageID(serverID) {
		return fmt.Errorf("%s: the fetched entity carries no numeric `id`, so a save would create a new one", command)
	}
	if given, ok := body["id"]; ok && !isZeroEngageID(given) && fmt.Sprint(given) != fmt.Sprint(serverID) {
		return fmt.Errorf("%s: --json carries `id` %v but %s has `id` %v; drop `id` or pass the matching entity", command, given, guid, serverID)
	}
	if value, present := body[field]; present && value != nil {
		given, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: `%s` must be a GUID string, got %s", command, field, jsonShapeName(value))
		}
		if strings.TrimSpace(given) != "" && !strings.EqualFold(strings.TrimSpace(given), guid) {
			return fmt.Errorf("%s: --json carries `%s` %q, which is not the entity being updated (%s)", command, field, given, guid)
		}
	}
	body["id"] = serverID
	body[field] = guid
	return nil
}

func finishEngageBody(body map[string]any, spec *engageWriteSpec) error {
	for arrayField, guidField := range spec.Nested {
		entries, ok := body[arrayField].([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			object, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if err := ensureEngageGUID(object, guidField); err != nil {
				return fmt.Errorf("%s[]: %w", arrayField, err)
			}
		}
	}
	if spec.Normalize != nil {
		return spec.Normalize(body)
	}
	return nil
}

// printEngageSave prints a save result. Engage answers either the saved
// entity, a wrapper with validationResults (personas, journeys), or a bare
// GUID string (goals, traffic filters); the last is turned into an object so
// the output stays a JSON object.
func printEngageSave(cmd *cobra.Command, deps Dependencies, verb string, result any, body map[string]any, field string, dryRun bool) error {
	if !dryRun {
		switch value := result.(type) {
		case string:
			result = map[string]any{verb: true, field: value}
		case nil:
			result = map[string]any{verb: true, field: body[field]}
		}
	}
	if err := printResult(cmd, deps, result); err != nil {
		return err
	}
	return engageValidationFailure(cmd, result)
}

// engageRejectedError is a save or delete Engage answered with 200 but a
// ValidationResultsModel saying it did not happen. It exits 4 like any other
// Management API refusal; the full response is printed first.
type engageRejectedError struct {
	command string
	errors  []string
}

func (e engageRejectedError) Error() string {
	if len(e.errors) == 0 {
		return e.command + ": Engage rejected the request (validationResults.isValid is false)"
	}
	return e.command + ": Engage rejected the request: " + strings.Join(e.errors, "; ")
}
func (engageRejectedError) ExitCode() int { return 4 }

func engageValidationFailure(cmd *cobra.Command, result any) error {
	object, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	validation := object
	if nested, ok := object["validationResults"].(map[string]any); ok {
		validation = nested
	}
	valid, ok := validation["isValid"].(bool)
	if !ok || valid {
		return nil
	}
	rejected := engageRejectedError{command: cmd.CommandPath()}
	if errs, ok := validation["errors"].([]any); ok {
		for _, item := range errs {
			rejected.errors = append(rejected.errors, fmt.Sprint(item))
		}
	}
	return rejected
}
