package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/validate"
)

type documentBulkUpdateItemResult struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type documentBulkUpdateResult struct {
	DryRun  bool                           `json:"dryRun"`
	Total   int                            `json:"total"`
	Updated int                            `json:"updated"`
	Skipped int                            `json:"skipped"`
	Failed  int                            `json:"failed"`
	Items   []documentBulkUpdateItemResult `json:"items"`
}

func fetchDocumentObject(ctx context.Context, client *api.Client, id string) (map[string]any, error) {
	result, err := client.Get(ctx, fmt.Sprintf("/document/%s", id), api.RequestOptions{})
	if err != nil {
		return nil, err
	}

	return decodeResult[map[string]any](result)
}

func documentPropertyPatch(alias string, rawValue string, rawValueJSON string) (map[string]any, error) {
	if err := requireValue("--property", alias); err != nil {
		return nil, err
	}
	if err := validate.String(alias); err != nil {
		return nil, err
	}

	hasValue := strings.TrimSpace(rawValue) != ""
	hasValueJSON := strings.TrimSpace(rawValueJSON) != ""
	if hasValue == hasValueJSON {
		return nil, fmt.Errorf("property updates require exactly one of --value or --value-json")
	}

	var value any
	if hasValueJSON {
		if err := json.Unmarshal([]byte(rawValueJSON), &value); err != nil {
			return nil, fmt.Errorf("invalid --value-json JSON: %w", err)
		}
	} else {
		if err := validate.String(rawValue); err != nil {
			return nil, err
		}
		value = rawValue
	}

	return map[string]any{
		"values": []any{
			map[string]any{
				"alias": alias,
				"value": value,
			},
		},
	}, nil
}

func loadDocumentIDs(ids []string, idFile string) ([]string, error) {
	unique := map[string]struct{}{}
	results := make([]string, 0, len(ids))

	appendID := func(raw string) error {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil
		}
		if err := validate.String(id); err != nil {
			return err
		}
		if _, exists := unique[id]; exists {
			return nil
		}
		unique[id] = struct{}{}
		results = append(results, id)
		return nil
	}

	for _, id := range ids {
		if err := appendID(id); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(idFile) == "" {
		return results, nil
	}
	if err := validate.String(idFile); err != nil {
		return nil, err
	}

	payload, err := os.ReadFile(idFile)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(payload), "\n") {
		if err := appendID(line); err != nil {
			return nil, err
		}
	}

	return results, nil
}

func executeDocumentBulkUpdate(ctx context.Context, client *api.Client, ids []string, fullBody map[string]any, mergePatch map[string]any, dryRun bool) documentBulkUpdateResult {
	result := documentBulkUpdateResult{
		DryRun: dryRun,
		Total:  len(ids),
		Items:  make([]documentBulkUpdateItemResult, 0, len(ids)),
	}

	for _, id := range ids {
		item := documentBulkUpdateItemResult{ID: id}

		body := fullBody
		if mergePatch != nil {
			current, err := fetchDocumentObject(ctx, client, id)
			if err != nil {
				item.Action = "fail"
				item.Error = err.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				continue
			}

			merged := mergeDatatypePayload(current, mergePatch)
			if reflect.DeepEqual(current, merged) {
				item.Action = "skip"
				item.Message = "already up to date"
				result.Skipped++
				result.Items = append(result.Items, item)
				continue
			}
			body = merged
		}

		if _, err := client.Put(ctx, fmt.Sprintf("/document/%s", id), body, api.RequestOptions{DryRun: dryRun}); err != nil {
			item.Action = "fail"
			item.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		item.Action = "update"
		if dryRun {
			item.Message = "validated"
		} else {
			item.Message = "updated"
		}
		result.Updated++
		result.Items = append(result.Items, item)
	}

	return result
}
