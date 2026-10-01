package commands

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// Umbraco Engage serves its own Management API mount, authorised with the
// same back-office bearer token as the core API. Its routes pass ids as
// query parameters (?id=...) rather than path segments, and the id kind is
// not uniform: most reads take an entity's GUID (`unique`, or `key` on goals
// and traffic filters) even when the parameter is named `id`, while A/B test
// reads take the numeric `id`. Every entity carries both, so each get
// command validates the kind up front and names the field to copy.
const engageAPIPrefix = "/umbraco/engage/management/api/v1"

func RegisterEngage(root *cobra.Command, deps Dependencies) {
	engage := &cobra.Command{
		Use:   "engage",
		Short: "Umbraco Engage operations (read-only: analytics, segments, personas, journeys, goals, A/B tests)",
		Long: "Read-only commands for the Umbraco Engage Management API (" + engageAPIPrefix + "). Requires Umbraco Engage on the target instance.\n\n" +
			"Engage entities carry two ids: a numeric `id` and a GUID `unique` (`key` on goals and traffic filters). Most get commands take the GUID; the A/B test reads take the numeric id. Each get command says which, and rejects the other kind before calling the API.\n\n" +
			"Start with 'umbraco engage status': it reports the license, the main switch, and whether Engage's data is reachable. When Engage's database migration is incomplete every data read answers HTTP 409 \"Umbraco Engage is unavailable\" (exit code 4).\n\n" +
			"Visitor profiles (/profile/*) are deliberately not exposed: they return personal data about individual visitors.",
	}
	engage.AddCommand(engageStatus(deps))
	engage.AddCommand(engageRead(deps, engageReadSpec{Use: "config", Short: "Show Engage's effective configuration (analytics, A/B testing, segmentation, reporting settings)", Path: "/configuration"}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "segment", Noun: "segment", Short: "Personalization segments",
		ListPath: "/segments/all", GetPath: "/segments", IDParam: "id", IDKind: engageGUID, IDField: "unique",
		ListFlags: []engageFlag{
			{Name: "temporary", Param: "isTemporary", Kind: engageFlagBool, Usage: "List temporary (unsaved) segments instead of saved ones"},
			{Name: "days", Param: "amountOfDays", Kind: engageFlagInt, Usage: "Window in days for the per-segment visitor statistics"},
		},
	}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "persona", Noun: "persona", Short: "Personas (implicit personalization)",
		ListPath: "/persona/all", GetPath: "/persona/details", IDParam: "id", IDKind: engageGUID, IDField: "unique",
	}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "journey", Noun: "customer journey", Short: "Customer journeys (implicit personalization)",
		ListPath: "/customer-journey/all", GetPath: "/customer-journey/details", IDParam: "id", IDKind: engageGUID, IDField: "unique",
	}))
	engage.AddCommand(engageGoal(deps))
	engage.AddCommand(engageABTest(deps))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "personalization", Noun: "applied personalization", Short: "Applied personalizations (segment-specific content variants)",
		ListPath: "/applied-personalization/all", GetPath: "/applied-personalization/id", IDParam: "id", IDKind: engageGUID, IDField: "unique",
	}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "campaign-group", Noun: "campaign group", Short: "Campaign groups (UTM campaign scoring)",
		ListPath: "/campaign-group/all", GetPath: "/campaign-group", IDParam: "id", IDKind: engageGUID, IDField: "unique",
	}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "referral-group", Noun: "referral group", Short: "Referral groups (referrer scoring)",
		ListPath: "/referral-group/all", GetPath: "/referral-group", IDParam: "id", IDKind: engageGUID, IDField: "unique",
	}))
	engage.AddCommand(engageEntityGroup(deps, engageEntity{
		Group: "traffic-filter", Noun: "traffic filter", Short: "Traffic filters (IP ranges excluded from analytics)",
		ListPath: "/traffic-filter/all", GetPath: "/traffic-filter", IDParam: "key", IDKind: engageGUID, IDField: "key",
	}))
	engage.AddCommand(engageAnalytics(deps))
	engage.AddCommand(engageAnnotation(deps))
	engage.AddCommand(engageStats(deps))
	engage.AddCommand(engageReporting(deps))
	root.AddCommand(engage)
}

func engageOpts(params map[string]any) api.RequestOptions {
	return api.RequestOptions{APIPrefix: engageAPIPrefix, Params: params}
}

// engageUnavailableHint is attached to the 409 Engage answers on every data
// route while its database schema migration is incomplete; the server's own
// detail says only "contact your administrator".
const engageUnavailableHint = "Engage is installed but disabled because its database schema alignment is incomplete; run 'umbraco engage status' and see https://docs.umbraco.com/umbraco-engage/upgrading/schema-alignment-guide"

// engageError adds Engage-specific hints to API errors while keeping the
// *api.APIError (and so exit code 4) intact.
func engageError(err error) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.StatusCode == http.StatusConflict:
		apiErr.Hint = engageUnavailableHint
	case apiErr.StatusCode == http.StatusNotFound && apiErr.Payload == nil:
		// A bare 404 is the route missing, which on this mount usually means
		// Engage is not installed at all.
		apiErr.Hint = "the Engage Management API route was not found; Umbraco Engage may not be installed on this instance"
	}
	return err
}

func engageGet(cmd *cobra.Command, deps Dependencies, path string, params map[string]any) (any, error) {
	result, err := deps.Client.Get(cmd.Context(), path, engageOpts(params))
	if err != nil {
		return nil, engageError(err)
	}
	return result, nil
}

type engageIDKind int

const (
	engageGUID engageIDKind = iota
	engageNumeric
)

// validateEngageID rejects the wrong id kind before the request: passing a
// numeric id where Engage expects a GUID (or the reverse) otherwise surfaces
// as a bare 400 model-binding error.
func validateEngageID(command string, value string, kind engageIDKind, field string, listCommand string) error {
	value = strings.TrimSpace(value)
	switch kind {
	case engageNumeric:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%s expects the numeric `%s` from '%s', got %q", command, field, listCommand, value)
		}
	default:
		if !isUUIDLike(value) {
			return fmt.Errorf("%s expects the GUID `%s` from '%s' (not the numeric `id`), got %q", command, field, listCommand, value)
		}
	}
	return nil
}

type engageFlagKind int

const (
	engageFlagString engageFlagKind = iota
	engageFlagInt
	engageFlagBool
)

// engageFlag maps a CLI flag onto one query parameter. Unset flags are not
// sent, so the server keeps its own defaults.
type engageFlag struct {
	Name  string
	Param string
	Kind  engageFlagKind
	Usage string
}

func bindEngageFlags(cmd *cobra.Command, flags []engageFlag) func() map[string]any {
	strs := map[string]*string{}
	ints := map[string]*int{}
	bools := map[string]*bool{}
	for _, flag := range flags {
		switch flag.Kind {
		case engageFlagInt:
			ints[flag.Name] = cmd.Flags().Int(flag.Name, 0, flag.Usage)
		case engageFlagBool:
			bools[flag.Name] = cmd.Flags().Bool(flag.Name, false, flag.Usage)
		default:
			strs[flag.Name] = cmd.Flags().String(flag.Name, "", flag.Usage)
		}
	}
	return func() map[string]any {
		var params map[string]any
		for _, flag := range flags {
			if !cmd.Flags().Changed(flag.Name) {
				continue
			}
			if params == nil {
				params = map[string]any{}
			}
			switch flag.Kind {
			case engageFlagInt:
				params[flag.Param] = *ints[flag.Name]
			case engageFlagBool:
				params[flag.Param] = *bools[flag.Name]
			default:
				params[flag.Param] = *strs[flag.Name]
			}
		}
		return params
	}
}

type engageReadSpec struct {
	Use   string
	Short string
	Long  string
	Path  string
	Flags []engageFlag
}

// engageRead builds an argument-free read with --fields projection. Engage
// list routes answer bare arrays without an {items,total} envelope and are
// not paged, so there are no pagination flags.
func engageRead(deps Dependencies, spec engageReadSpec) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: spec.Use, Short: spec.Short, Long: spec.Long, Args: cobra.NoArgs}
	params := bindEngageFlags(cmd, spec.Flags)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		result, err := engageGet(cmd, deps, spec.Path, params())
		if err != nil {
			return err
		}
		return printResult(cmd, deps, applyFieldsProjection(result, fields))
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

type engageGetSpec struct {
	Use         string
	Short       string
	Long        string
	Path        string
	IDParam     string
	IDKind      engageIDKind
	IDField     string
	ListCommand string
}

func engageGetByID(deps Dependencies, spec engageGetSpec) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateEngageID("engage "+cmd.Parent().Name()+" "+cmd.Name(), args[0], spec.IDKind, spec.IDField, spec.ListCommand); err != nil {
				return err
			}
			result, err := engageGet(cmd, deps, spec.Path, map[string]any{spec.IDParam: strings.TrimSpace(args[0])})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	return cmd
}

type engageEntity struct {
	Group     string
	Noun      string
	Short     string
	ListPath  string
	ListFlags []engageFlag
	GetPath   string
	IDParam   string
	IDKind    engageIDKind
	IDField   string
}

// engageEntityGroup builds the list/get pair shared by the configuration
// entities (segments, personas, journeys, ...).
func engageEntityGroup(deps Dependencies, entity engageEntity) *cobra.Command {
	group := &cobra.Command{Use: entity.Group, Short: entity.Short}
	listCommand := "umbraco engage " + entity.Group + " list"
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use:   "list",
		Short: fmt.Sprintf("List every %s (GET %s)", entity.Noun, entity.ListPath),
		Long:  fmt.Sprintf("GET %s. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `%s`; 'get' takes the `%s`.", entity.ListPath, entity.IDField, entity.IDField),
		Path:  entity.ListPath,
		Flags: entity.ListFlags,
	}))
	group.AddCommand(engageGetByID(deps, engageGetSpec{
		Use:         "get <" + entity.IDField + ">",
		Short:       fmt.Sprintf("Get one %s by its GUID `%s`", entity.Noun, entity.IDField),
		Long:        fmt.Sprintf("GET %s?%s=<%s>. Pass the GUID `%s` from '%s', not the numeric `id`.", entity.GetPath, entity.IDParam, entity.IDField, entity.IDField, listCommand),
		Path:        entity.GetPath,
		IDParam:     entity.IDParam,
		IDKind:      entity.IDKind,
		IDField:     entity.IDField,
		ListCommand: listCommand,
	}))
	return group
}

func engageGoal(deps Dependencies) *cobra.Command {
	group := &cobra.Command{Use: "goal", Short: "Goals (conversions tracked by analytics, A/B tests and scoring)"}
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "list", Short: "List every goal (GET /goals/all)",
		Long: "GET /goals/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `key`; 'goal get' takes the `key`.",
		Path: "/goals/all",
	}))
	group.AddCommand(engageRead(deps, engageReadSpec{Use: "main", Short: "List the main (macro) goals (GET /goals/main)", Path: "/goals/main"}))
	group.AddCommand(engageRead(deps, engageReadSpec{Use: "types", Short: "List the goal types goals can be configured with (GET /goal/all/types)", Path: "/goal/all/types"}))
	group.AddCommand(engageGetByID(deps, engageGetSpec{
		Use: "get <key>", Short: "Get one goal with its full configuration by its GUID `key`",
		Long: "GET /goal/details?id=<key>. Pass the GUID `key` from 'umbraco engage goal list' (the detail model calls it `unique`), not the numeric `id`.",
		Path: "/goal/details", IDParam: "id", IDKind: engageGUID, IDField: "key", ListCommand: "umbraco engage goal list",
	}))
	return group
}

func engageABTest(deps Dependencies) *cobra.Command {
	group := &cobra.Command{
		Use:   "abtest",
		Short: "A/B tests, their variants, and A/B test projects",
		Long:  "A/B test reads. Unlike the rest of Engage, 'abtest get' and 'abtest variants' take the numeric `id`; 'abtest project' takes the project's GUID `unique`.",
	}
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "list", Short: "List every A/B test (GET /ab-test/all)",
		Long: "GET /ab-test/all. Returns a bare array, not paged. 'abtest get' and 'abtest variants' take an entry's numeric `id`.",
		Path: "/ab-test/all",
	}))
	group.AddCommand(engageGetByID(deps, engageGetSpec{
		Use: "get <id>", Short: "Get one A/B test by its numeric `id`",
		Long: "GET /ab-test?id=<id>. Takes the numeric `id` from 'umbraco engage abtest list', not the GUID `unique`.",
		Path: "/ab-test", IDParam: "id", IDKind: engageNumeric, IDField: "id", ListCommand: "umbraco engage abtest list",
	}))
	group.AddCommand(engageGetByID(deps, engageGetSpec{
		Use: "variants <abTestId>", Short: "List the variants of one A/B test by the test's numeric `id`",
		Long: "GET /ab-test-variant/all?abTestId=<id>. Takes the numeric `id` from 'umbraco engage abtest list'.",
		Path: "/ab-test-variant/all", IDParam: "abTestId", IDKind: engageNumeric, IDField: "id", ListCommand: "umbraco engage abtest list",
	}))
	group.AddCommand(engageRead(deps, engageReadSpec{
		Use: "projects", Short: "List every A/B test project (GET /ab-test-project/all)",
		Long: "GET /ab-test-project/all. A project groups the A/B tests run on one page; 'abtest project' takes an entry's GUID `unique`.",
		Path: "/ab-test-project/all",
	}))
	group.AddCommand(engageGetByID(deps, engageGetSpec{
		Use: "project <unique>", Short: "Get one A/B test project with its tests by the project's GUID `unique`",
		Long: "GET /ab-test-project/details?id=<unique>. Takes the GUID `unique` from 'umbraco engage abtest projects', not the numeric `id`.",
		Path: "/ab-test-project/details", IDParam: "id", IDKind: engageGUID, IDField: "unique", ListCommand: "umbraco engage abtest projects",
	}))
	return group
}

// engageDataProbePath is a cheap data route used by 'engage status' to tell
// "installed and serving data" from "installed but disabled" (409).
const engageDataProbePath = "/goal/all/types"

func engageStatus(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report Engage's version, license, main switch, add-ons, and whether its data is reachable",
		Long: "Combines GET /package, /main-switch and /add-ons with a probe of a data route (" + engageDataProbePath + "). " +
			"`dataAvailable` is false when Engage answers 409 \"Umbraco Engage is unavailable\", which it does on every data route while its database schema alignment is incomplete; `unavailable` then carries the server's title and detail. " +
			"Any other failure (auth, a missing route because Engage is not installed) is returned as an error with its exit code.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := engageGet(cmd, deps, "/package", nil)
			if err != nil {
				return err
			}
			mainSwitch, err := engageGet(cmd, deps, "/main-switch", nil)
			if err != nil {
				return err
			}
			addOns, err := engageGet(cmd, deps, "/add-ons", nil)
			if err != nil {
				return err
			}
			status := map[string]any{
				"package":       pkg,
				"mainSwitch":    mainSwitch,
				"addOns":        addOns,
				"dataAvailable": true,
			}
			if _, err := deps.Client.Get(cmd.Context(), engageDataProbePath, engageOpts(nil)); err != nil {
				var apiErr *api.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict {
					return engageError(err)
				}
				status["dataAvailable"] = false
				unavailable := map[string]any{"hint": engageUnavailableHint}
				if problem, ok := apiErr.Payload.(map[string]any); ok {
					unavailable["title"] = problem["title"]
					unavailable["detail"] = problem["detail"]
				}
				status["unavailable"] = unavailable
			}
			return printResult(cmd, deps, status)
		},
	}
}
