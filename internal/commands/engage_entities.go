package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Write specs for the Engage configuration entities. Scaffolds mirror the
// back office's create scaffolds (Engage 18.1.0 App_Plugins/Umbraco.Engage
// detail repositories) minus their UI-only fields; Nested lists the arrays
// whose entries the save models require a GUID on.

var engageSegmentWrite = &engageWriteSpec{
	SavePath: "/segments", DeletePath: "/segments", DeleteParam: "id", BodyIDField: "unique",
	Scaffold: func() map[string]any {
		return map[string]any{"name": "", "description": "", "isTemporary": false, "sortOrder": 0, "controlGroupSize": 0.2, "rules": []any{}}
	},
	Nested:    map[string]string{"rules": "unique"},
	Normalize: normalizeEngageSegment,
	Notes:     " `controlGroupSize` is a fraction (0.2 = 20%), not the percentage the back office displays; rules come from 'umbraco engage segment get' on an existing segment.",
}

// normalizeEngageSegment rejects a percentage-style controlGroupSize: the
// back office shows 20 and sends 0.2, so 20 on the wire would mean 2000%.
func normalizeEngageSegment(body map[string]any) error {
	value, ok := body["controlGroupSize"]
	if !ok || value == nil {
		return nil
	}
	size, ok := value.(float64)
	if !ok {
		return fmt.Errorf("`controlGroupSize` must be a number between 0 and 1, got %v", value)
	}
	if size < 0 || size > 1 {
		return fmt.Errorf("`controlGroupSize` is a fraction between 0 and 1 (0.2 = 20%%), got %v", size)
	}
	return nil
}

var engagePersonaWrite = &engageWriteSpec{
	SavePath: "/persona", DeletePath: "/persona", DeleteParam: "id", BodyIDField: "unique", EmptyPath: "/persona/empty",
	Scaffold: func() map[string]any { return map[string]any{"title": "", "description": "", "personas": []any{}} },
	Nested:   map[string]string{"personas": "unique"},
	Notes:    " A persona group holds its personas in `personas[]`; Engage answers {persona, validationResults} and a false `isValid` exits 4.",
}

var engageJourneyWrite = &engageWriteSpec{
	SavePath: "/customer-journey", DeletePath: "/customer-journey", DeleteParam: "id", BodyIDField: "unique", EmptyPath: "/customer-journey/empty",
	Scaffold: func() map[string]any { return map[string]any{"title": "", "description": "", "steps": []any{}} },
	Nested:   map[string]string{"steps": "unique"},
	Notes:    " A journey holds its steps in `steps[]`; Engage answers {journey, validationResults} and a false `isValid` exits 4.",
}

var engagePersonalizationWrite = &engageWriteSpec{
	SavePath: "/applied-personalization", DeletePath: "/applied-personalization", DeleteParam: "id", BodyIDField: "unique",
	Scaffold: func() map[string]any {
		return map[string]any{"name": "", "description": "", "type": "MultiPage", "isActive": true, "pages": []any{}, "contentTypes": []any{}}
	},
	Nested:    map[string]string{"pages": "unique", "contentTypes": "key"},
	Normalize: normalizeEngagePersonalization,
	Notes:     " `segmentId` is the segment's numeric `id`. Like the back office, a `ContentType` personalization is saved with `pages` emptied and any other type with `contentTypes` emptied.",
}

func normalizeEngagePersonalization(body map[string]any) error {
	if body["type"] == "ContentType" {
		body["pages"] = []any{}
	} else {
		body["contentTypes"] = []any{}
	}
	return nil
}

func engageScoringGroupScaffold(items string) func() map[string]any {
	return func() map[string]any {
		return map[string]any{"name": "", "description": "", "invalid": false, items: []any{}, "customerJourneyScoring": []any{}, "personaScoring": []any{}}
	}
}

var engageCampaignGroupWrite = &engageWriteSpec{
	SavePath: "/campaign-group", DeletePath: "/campaign-group", DeleteParam: "id", BodyIDField: "unique",
	Scaffold: engageScoringGroupScaffold("campaigns"),
	Nested:   map[string]string{"campaigns": "unique", "customerJourneyScoring": "unique", "personaScoring": "unique"},
	Notes:    " `campaigns[]` holds the UTM matches (utmSource, utmMedium, utmCampaign, ...); deleting a group reverts its campaigns to unscored.",
}

var engageReferralGroupWrite = &engageWriteSpec{
	SavePath: "/referral-group", DeletePath: "/referral-group", DeleteParam: "id", BodyIDField: "unique",
	Scaffold: engageScoringGroupScaffold("pages"),
	Nested:   map[string]string{"pages": "key", "customerJourneyScoring": "unique", "personaScoring": "unique"},
	Notes:    " `pages[]` holds the referring pages (pageUrl, domainOnly).",
}

var engageTrafficFilterWrite = &engageWriteSpec{
	SavePath: "/traffic-filter", DeletePath: "/traffic-filter", DeleteParam: "key", BodyIDField: "key", EmptyPath: "/traffic-filter/empty",
	Scaffold: func() map[string]any {
		return map[string]any{"name": "", "description": "", "isActive": true, "values": []any{}}
	},
	Notes: " `mode` is Block, Filter or BlockAndFilter. Engage answers the saved filter's `key`.",
}

var engageGoalWrite = &engageWriteSpec{
	SavePath: "/goal", BodyIDField: "unique",
	Scaffold: func() map[string]any {
		return map[string]any{
			"name": "", "value": 0, "goalTypeConfig": "", "isMain": true, "isInverted": false, "isActive": false, "isInvalid": false,
			"isImplicitScoringEnabled": false, "implicitPersonaScoring": []any{}, "implicitCustomerJourneyStepScoring": []any{},
		}
	},
	Nested:    map[string]string{"implicitPersonaScoring": "unique", "implicitCustomerJourneyStepScoring": "unique"},
	Normalize: normalizeEngageGoal,
	Notes: " `goalTypeId` comes from 'umbraco engage goal types'. Like the back office, `isActive` defaults to false. Engage answers the goal's GUID. " +
		"Engage's Management API has no goal delete route; deactivate a goal with --merge-json '{\"isActive\":false}'.",
}

// normalizeEngageGoal fills what the back office fills before saving a
// goal: the scoring arrays are required (the detail model may answer null)
// and each scoring entry is a GoalCompletion score.
func normalizeEngageGoal(body map[string]any) error {
	for _, field := range []string{"implicitPersonaScoring", "implicitCustomerJourneyStepScoring"} {
		entries, ok := body[field].([]any)
		if !ok {
			body[field] = []any{}
			continue
		}
		for _, entry := range entries {
			if object, ok := entry.(map[string]any); ok {
				if _, set := object["scoreType"]; !set {
					object["scoreType"] = "GoalCompletion"
				}
			}
		}
	}
	return nil
}

// engageSegmentUpdatePriority wraps POST /segments/update-priority, whose
// body is a bare array of {id, sortOrder} with the numeric segment ids.
func engageSegmentUpdatePriority(deps Dependencies) *cobra.Command {
	var jsonPayload, order string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update-priority",
		Short: "Reorder segment priority (POST /segments/update-priority)",
		Long: "POST /segments/update-priority with a bare array of {id, sortOrder}. A visitor in several segments gets the content of the highest-priority one. " +
			"--order 7,3,9 lists numeric segment `id`s (from 'umbraco engage segment list') highest priority first and sends sortOrder 0, 1, 2, ...; --json sends an array verbatim.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := engagePriorityBody(jsonPayload, order)
			if err != nil {
				return err
			}
			result, err := deps.Client.Post(cmd.Context(), "/segments/update-priority", body, engageWriteOpts(nil, dryRun))
			if err != nil {
				return engageError(err)
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&order, "order", "", "Comma-separated numeric segment ids, highest priority first")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Raw [{\"id\":7,\"sortOrder\":0},...] array")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func engagePriorityBody(jsonPayload string, order string) ([]any, error) {
	hasJSON := strings.TrimSpace(jsonPayload) != ""
	if hasJSON == (strings.TrimSpace(order) != "") {
		return nil, fmt.Errorf("update-priority requires exactly one of --order or --json")
	}
	if hasJSON {
		var body []any
		if err := json.Unmarshal([]byte(jsonPayload), &body); err != nil {
			return nil, fmt.Errorf("--json must be a JSON array of {\"id\":<number>,\"sortOrder\":<number>}: %w", err)
		}
		for i, entry := range body {
			object, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("--json[%d] must be an object with `id` and `sortOrder`", i)
			}
			if _, ok := object["id"].(float64); !ok {
				return nil, fmt.Errorf("--json[%d].id must be the numeric segment `id`", i)
			}
			if _, ok := object["sortOrder"].(float64); !ok {
				return nil, fmt.Errorf("--json[%d].sortOrder must be a number", i)
			}
		}
		return body, nil
	}
	parts := strings.Split(order, ",")
	body := make([]any, 0, len(parts))
	seen := map[int64]bool{}
	for i, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--order expects numeric segment `id`s from 'umbraco engage segment list' (not GUIDs), got %q", strings.TrimSpace(part))
		}
		if seen[id] {
			return nil, fmt.Errorf("--order lists segment %d twice", id)
		}
		seen[id] = true
		body = append(body, map[string]any{"id": id, "sortOrder": i})
	}
	return body, nil
}
