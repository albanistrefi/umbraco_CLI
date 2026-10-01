package commands

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Site-wide Engage actions: the main switch, reporting regeneration and
// analytics annotations. The switch and the regeneration affect the whole
// site rather than one entity, so both are gated like hard deletes.

func engageMainSwitch(deps Dependencies) *cobra.Command {
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

func engageSwitchAction(deps Dependencies, use string, path string, short string, consequence string) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  "POST " + path + " (no body), then reads GET /main-switch back. Requires --force (or --dry-run to rehearse).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, consequence, force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), path, nil, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			if dryRun {
				return printResult(cmd, deps, result)
			}
			state, err := engageGet(cmd, deps, "/main-switch", nil)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, map[string]any{"mainSwitch": state})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the site-wide switch")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

// engageReportingGenerationTitle is the 409 title Engage answers on every
// data route while its schema alignment is incomplete; any other 409 from
// the generation start means a generation is already running.
const engageReportingGenerationTitle = "Umbraco Engage is unavailable"

func engageReportingGenerate(deps Dependencies) *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Regenerate the reporting tables (POST /reporting/generation/start)",
		Long: "POST /reporting/generation/start (no body) starts a regeneration of Engage's aggregated reporting tables in the background; follow it with 'umbraco engage reporting status'. " +
			"The back office warns that regenerating can affect site performance, so this requires --force (or --dry-run to rehearse). A 409 other than \"Umbraco Engage is unavailable\" means a generation is already running.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "regenerates every reporting table, which can slow the site while it runs", force, dryRun); err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/reporting/generation/start", nil, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageReportingError(engageError(err))
			}
			return printMutationResult(cmd, deps, "started", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm the regeneration")
	addDryRunFlag(cmd, &dryRun)
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

func engageAnnotationCreate(deps Dependencies) *cobra.Command {
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
				return printResult(cmd, deps, result)
			}
			if err := requireValue("--json", jsonPayload); err != nil {
				return err
			}
			body, err := parsePayload(jsonPayload)
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
			result, err := deps.Client.Post(cmd.Context(), "/annotations", body, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			return printMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Annotation payload as JSON")
	cmd.Flags().BoolVar(&printTemplate, "print-template", false, "Print the server's blank annotation; edit it and pass it to --json")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func engageAnnotationDelete(deps Dependencies) *cobra.Command {
	return engageDelete(deps, engageDeleteSpec{
		Use:         "delete <id>",
		Short:       "Permanently delete an annotation by its numeric `id`",
		Long:        "DELETE /annotations?id=<id>. Annotations carry only a numeric `id` (from 'umbraco engage annotation list'). Requires --force (or --dry-run to rehearse).",
		Path:        "/annotations",
		Param:       "id",
		IDKind:      engageNumeric,
		IDField:     "id",
		ListCommand: "umbraco engage annotation list",
		Consequence: "permanently deletes the annotation",
	})
}
