package engage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/jsonvalue"
	"umbraco-cli/internal/uuid"
)

// Site-wide Engage actions: the main switch, reporting regeneration and
// analytics annotations. The switch and the regeneration affect the whole
// site rather than one entity, so both are gated like hard deletes.

func engageMainSwitch(deps cmdkit.Dependencies) *cobra.Command {
	group := &cobra.Command{
		Use:   "main-switch",
		Short: "Engage's site-wide main switch (get, on, off)",
		Long:  "The main switch turns Engage on or off for the whole site; the back office calls this enabling and disabling Engage. 'on' and 'off' require --force (or --dry-run to rehearse).",
	}
	group.AddCommand(engageRead(deps, engageReadSpec{Use: "get", Short: "Show whether the main switch is on (GET /main-switch)", Path: "/main-switch"}))
	group.AddCommand(engageSwitchAction(deps, "on", "/main-switch/turn-on", "Turn Engage on site-wide (POST /main-switch/turn-on)", "turns Engage on for the whole site"))
	group.AddCommand(engageSwitchAction(deps, "off", "/main-switch/turn-off", "Turn Engage off site-wide (POST /main-switch/turn-off)", "turns Engage off for the whole site"))
	return group
}

func engageSwitchAction(deps cmdkit.Dependencies, use string, path string, short string, consequence string) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  "POST " + path + " (no body), then reads GET /main-switch back. Requires --force (or --dry-run to rehearse).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireForceOrDryRun(cmd, consequence, force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), path, nil, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			if dryRun {
				return cmdkit.PrintResult(cmd, deps, result)
			}
			state, err := engageGet(cmd, deps, "/main-switch", nil)
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, map[string]any{"mainSwitch": state})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the site-wide switch")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// engageReportingGenerationTitle is the 409 title Engage answers on every
// data route while its schema alignment is incomplete; any other 409 from
// the generation start means a generation is already running.
const engageReportingGenerationTitle = "Umbraco Engage is unavailable"

func engageReportingGenerate(deps cmdkit.Dependencies) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Regenerate the reporting tables (POST /reporting/generation/start)",
		Long: "POST /reporting/generation/start (no body) starts a regeneration of Engage's aggregated reporting tables in the background; follow it with 'umbraco engage reporting status'. " +
			"The back office warns that regenerating can affect site performance, so this requires --force (or --dry-run to rehearse). A 409 other than \"Umbraco Engage is unavailable\" means a generation is already running.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdkit.RequireForceOrDryRun(cmd, "regenerates every reporting table, which can slow the site while it runs", force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/reporting/generation/start", nil, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageReportingError(engageError(err))
			}
			return cmdkit.PrintMutationResult(cmd, deps, "started", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the regeneration")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

func engageReportingError(err error) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
		return err
	}
	if problem, ok := apiErr.Payload.(map[string]any); ok && problem["title"] != engageReportingGenerationTitle {
		apiErr.Hint = "a reporting generation is probably already running; check 'umbraco engage reporting status'"
	}
	return err
}

func engageAnnotationCreate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun, printTemplate bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an analytics annotation (POST /annotations)",
		Long: "POST /annotations with `id` 0. Needs `timestamp` (RFC 3339), `description` and `visibility` (Always, Node, NodeAndDescendants, Created, Published, AbTestStart, AbTestEnd); " +
			"anything but Always pins it to `pageVariants` [{\"unique\":<document GUID>,\"culture\":\"\"}]. --print-template prints GET /annotations/empty, the server's blank template.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printTemplate {
				result, err := engageGet(cmd, deps, "/annotations/empty", nil)
				if err != nil {
					return err
				}
				return cmdkit.PrintResult(cmd, deps, result)
			}
			if err := cmdkit.RequireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := cmdkit.ParsePayload(jsonPayload)
			if err != nil {
				return err
			}
			if id, ok := body["id"]; ok && !isZeroEngageID(id) {
				return fmt.Errorf("engage annotation create sends `id` 0; --json carries `id` %v, which would overwrite that annotation", id)
			}
			body["id"] = 0
			if _, ok := body["pageVariants"]; !ok {
				body["pageVariants"] = []any{}
			}
			if err := validateEngageAnnotation(body); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/annotations", body, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			return cmdkit.PrintMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Annotation payload as JSON")
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print the server's blank annotation; edit it and pass it to --json")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// The annotation table's timestamp is a SQL datetime, so these are the
// first and last values it can hold; Engage answers 500 for a range that
// starts earlier.
const (
	engageAnnotationEarliest = "1753-01-01T00:00:00Z"
	engageAnnotationLatest   = "9999-12-31T23:59:59.997Z"
)

// engageAnnotationDelete does not use engageDelete: Engage's DELETE
// /annotations only marks a stored row invalid and answers 200 whether or not
// the id exists, so the command checks the annotation is listed before the
// DELETE and gone after it.
func engageAnnotationDelete(deps cmdkit.Dependencies) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Hide (soft-delete) a stored annotation by its numeric `id`",
		Long: "DELETE /annotations?id=<id>. Engage soft-deletes: it marks the annotation invalid, so it is no longer listed, but keeps its row. Requires --force (or --dry-run to rehearse).\n\n" +
			"Only stored annotations, made with 'umbraco engage annotation create' or in the back office, have an `id`. The ones Engage generates from A/B tests (started, stopped) and page history (published, created) list with `id` 0 and cannot be deleted, so an id of 0 or less is refused.\n\n" +
			"Engage answers 200 whether or not the id exists, so the command first looks the id up among the stored annotations (GET /annotations/all from " + engageAnnotationEarliest + " to " + engageAnnotationLatest + ") and refuses one it cannot find. " +
			"After the DELETE it lists them again and reports success only when the annotation is gone. Either failure exits 4. --dry-run runs the lookup and prints the planned DELETE.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := "engage annotation delete"
			listCommand := "umbraco engage annotation list"
			if err := validateEngageID(command, args[0], engageNumeric, "id", listCommand); err != nil {
				return err
			}
			id, _ := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if id <= 0 {
				return fmt.Errorf("%s: id %d is not a stored annotation. Annotations Engage generates from A/B tests (started, stopped) and page history (published, created) list with `id` 0 and cannot be deleted", command, id)
			}
			if err := cmdkit.RequireForceOrDryRun(cmd, "hides (soft-deletes) the annotation", force, dryRun); err != nil {
				return err
			}
			if _, err := engageFindStoredAnnotation(cmd.Context(), deps, id); err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), "/annotations", engageWriteOpts(map[string]any{"id": strconv.FormatInt(id, 10)}, dryRun))
			if err != nil {
				return engageError(err)
			}
			if dryRun {
				return cmdkit.PrintResult(cmd, deps, result)
			}
			if _, err := engageFindStoredAnnotation(cmd.Context(), deps, id); err == nil {
				return engageAnnotationError(fmt.Sprintf("%s: Engage answered the DELETE for annotation %d, but it is still listed", command, id))
			} else if !errors.As(err, new(engageAnnotationError)) {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, map[string]any{"deleted": true, "id": id})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm hiding the annotation")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

// engageAnnotationError is an annotation delete that Engage's answers show
// did not or cannot happen. It exits 4 like a Management API refusal.
type engageAnnotationError string

func (e engageAnnotationError) Error() string { return string(e) }
func (engageAnnotationError) ExitCode() int   { return 4 }

// engageFindStoredAnnotation returns the listed, non-invalid annotation with
// id, or an engageAnnotationError when there is none.
func engageFindStoredAnnotation(ctx context.Context, deps cmdkit.Dependencies, id int64) (map[string]any, error) {
	result, err := deps.Client.Get(ctx, "/annotations/all", engageOpts(map[string]any{"from": engageAnnotationEarliest, "to": engageAnnotationLatest}))
	if err != nil {
		return nil, engageError(err)
	}
	entries, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("GET /annotations/all: expected an array, got %s", jsonvalue.ShapeName(result))
	}
	want := strconv.FormatInt(id, 10)
	for _, entry := range entries {
		annotation, ok := entry.(map[string]any)
		if !ok || jsonvalue.Text(annotation["id"]) != want || annotation["invalid"] == true {
			continue
		}
		return annotation, nil
	}
	return nil, engageAnnotationError(fmt.Sprintf("engage annotation delete: no stored annotation with id %d (see 'umbraco engage annotation list')", id))
}

// engageAnnotationVisibilities is the AnnotationVisibilityModel enum
// (Engage 18.1.0). Every value but Always pins the annotation to pages.
var engageAnnotationVisibilities = []string{"Always", "Node", "NodeAndDescendants", "Created", "Published", "AbTestStart", "AbTestEnd"}

// validateEngageAnnotation checks the fields the save model requires before
// the POST, so --dry-run rehearses the real request rather than one the
// server would reject. It canonicalises the visibility spelling in place.
func validateEngageAnnotation(body map[string]any) error {
	description, ok := body["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return fmt.Errorf("annotation create needs a non-empty string `description`")
	}
	timestamp, ok := body["timestamp"].(string)
	if !ok {
		return fmt.Errorf("annotation create needs a `timestamp` string (RFC 3339 date-time)")
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(timestamp)); err != nil {
		return fmt.Errorf("`timestamp` must be an RFC 3339 date-time such as 2026-09-01T00:00:00Z, got %q", timestamp)
	}
	visibility, ok := body["visibility"].(string)
	if !ok {
		return fmt.Errorf("annotation create needs a `visibility` string, one of %s", strings.Join(engageAnnotationVisibilities, ", "))
	}
	canonical := ""
	for _, name := range engageAnnotationVisibilities {
		if strings.EqualFold(strings.TrimSpace(visibility), name) {
			canonical = name
		}
	}
	if canonical == "" {
		return fmt.Errorf("`visibility` %q is not one of %s", visibility, strings.Join(engageAnnotationVisibilities, ", "))
	}
	body["visibility"] = canonical
	variants, ok := body["pageVariants"].([]any)
	if !ok {
		return fmt.Errorf("`pageVariants` must be an array of {\"unique\":<document GUID>,\"culture\":\"\"}")
	}
	if canonical != "Always" && len(variants) == 0 {
		return fmt.Errorf("visibility %s pins the annotation to pages, so `pageVariants` needs at least one {\"unique\":<document GUID>,\"culture\":\"\"}", canonical)
	}
	for i, entry := range variants {
		variant, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("pageVariants[%d] must be an object, got %s", i, jsonvalue.ShapeName(entry))
		}
		if unique, ok := variant["unique"].(string); !ok || !uuid.Valid(unique) {
			return fmt.Errorf("pageVariants[%d].unique must be a document GUID", i)
		}
		if culture, present := variant["culture"]; present && culture != nil {
			if _, ok := culture.(string); !ok {
				return fmt.Errorf("pageVariants[%d].culture must be a string or null", i)
			}
		}
	}
	return nil
}
