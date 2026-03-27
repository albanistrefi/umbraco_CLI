package commands

import (
	"context"
	"encoding/csv"
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

type documentCSVFieldMapping struct {
	Alias  string `json:"alias"`
	Column string `json:"column"`
}

type documentCSVUpdateOptions struct {
	File     string
	IDColumn string
	Mappings []documentCSVFieldMapping
	DryRun   bool
}

type documentCSVUpdateItemResult struct {
	Row     int      `json:"row"`
	ID      string   `json:"id"`
	Action  string   `json:"action"`
	Aliases []string `json:"aliases,omitempty"`
	Message string   `json:"message,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type documentCSVUpdateResult struct {
	File      string                        `json:"file"`
	DryRun    bool                          `json:"dryRun"`
	TotalRows int                           `json:"totalRows"`
	Updated   int                           `json:"updated"`
	Skipped   int                           `json:"skipped"`
	Failed    int                           `json:"failed"`
	Items     []documentCSVUpdateItemResult `json:"items"`
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

func parseDocumentCSVFieldMappings(properties []string, rawMappings []string) ([]documentCSVFieldMapping, error) {
	results := make([]documentCSVFieldMapping, 0, len(properties)+len(rawMappings))
	seen := map[string]struct{}{}

	appendMapping := func(alias string, column string) error {
		alias = strings.TrimSpace(alias)
		column = strings.TrimSpace(column)
		if err := requireValue("alias", alias); err != nil {
			return err
		}
		if err := requireValue("column", column); err != nil {
			return err
		}
		if err := validate.String(alias); err != nil {
			return err
		}
		if err := validate.String(column); err != nil {
			return err
		}
		if _, exists := seen[alias]; exists {
			return fmt.Errorf("duplicate CSV mapping for alias %q", alias)
		}
		seen[alias] = struct{}{}
		results = append(results, documentCSVFieldMapping{Alias: alias, Column: column})
		return nil
	}

	for _, property := range properties {
		if err := appendMapping(property, property); err != nil {
			return nil, err
		}
	}

	for _, raw := range rawMappings {
		alias, column, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --field mapping %q (expected alias=column)", raw)
		}
		if err := appendMapping(alias, column); err != nil {
			return nil, err
		}
	}

	return results, nil
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

		requestOptions := api.RequestOptions{DryRun: dryRun}
		if mergePatch != nil {
			requestOptions.SkipValidation = true
		}
		if _, err := client.Put(ctx, fmt.Sprintf("/document/%s", id), body, requestOptions); err != nil {
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

func executeDocumentCSVUpdate(ctx context.Context, client *api.Client, opts documentCSVUpdateOptions) (documentCSVUpdateResult, error) {
	if err := validate.String(opts.File); err != nil {
		return documentCSVUpdateResult{}, err
	}
	if err := validate.String(opts.IDColumn); err != nil {
		return documentCSVUpdateResult{}, err
	}

	file, err := os.Open(opts.File)
	if err != nil {
		return documentCSVUpdateResult{}, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return documentCSVUpdateResult{}, err
	}
	if len(records) < 1 {
		return documentCSVUpdateResult{}, fmt.Errorf("CSV file %s is empty", opts.File)
	}

	headers := records[0]
	headerIndex := map[string]int{}
	for index, header := range headers {
		name := strings.TrimSpace(header)
		if name == "" {
			return documentCSVUpdateResult{}, fmt.Errorf("CSV file %s contains an empty header", opts.File)
		}
		if _, exists := headerIndex[name]; exists {
			return documentCSVUpdateResult{}, fmt.Errorf("CSV file %s contains duplicate header %q", opts.File, name)
		}
		headerIndex[name] = index
	}

	idIndex, ok := headerIndex[opts.IDColumn]
	if !ok {
		return documentCSVUpdateResult{}, fmt.Errorf("CSV file %s is missing id column %q", opts.File, opts.IDColumn)
	}
	for _, mapping := range opts.Mappings {
		if _, ok := headerIndex[mapping.Column]; !ok {
			return documentCSVUpdateResult{}, fmt.Errorf("CSV file %s is missing mapped column %q", opts.File, mapping.Column)
		}
	}

	result := documentCSVUpdateResult{
		File:      opts.File,
		DryRun:    opts.DryRun,
		TotalRows: len(records) - 1,
		Items:     make([]documentCSVUpdateItemResult, 0, len(records)-1),
	}

	seenIDs := map[string]int{}
	for rowIndex, record := range records[1:] {
		resultItem := documentCSVUpdateItemResult{Row: rowIndex + 2}
		if len(record) < len(headers) {
			padded := make([]string, len(headers))
			copy(padded, record)
			record = padded
		}

		id := strings.TrimSpace(record[idIndex])
		resultItem.ID = id
		if id == "" {
			resultItem.Action = "fail"
			resultItem.Error = "blank document ID"
			result.Failed++
			result.Items = append(result.Items, resultItem)
			continue
		}
		if err := validate.String(id); err != nil {
			resultItem.Action = "fail"
			resultItem.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, resultItem)
			continue
		}
		if firstRow, exists := seenIDs[id]; exists {
			resultItem.Action = "fail"
			resultItem.Error = fmt.Sprintf("duplicate document ID also seen on row %d", firstRow)
			result.Failed++
			result.Items = append(result.Items, resultItem)
			continue
		}
		seenIDs[id] = resultItem.Row

		values := make([]any, 0, len(opts.Mappings))
		aliases := make([]string, 0, len(opts.Mappings))
		for _, mapping := range opts.Mappings {
			columnIndex := headerIndex[mapping.Column]
			cell := strings.TrimSpace(record[columnIndex])
			if cell == "" {
				continue
			}
			values = append(values, map[string]any{
				"alias": mapping.Alias,
				"value": cell,
			})
			aliases = append(aliases, mapping.Alias)
		}
		resultItem.Aliases = aliases
		if len(values) == 0 {
			resultItem.Action = "skip"
			resultItem.Message = "no mapped values"
			result.Skipped++
			result.Items = append(result.Items, resultItem)
			continue
		}

		current, err := fetchDocumentObject(ctx, client, id)
		if err != nil {
			resultItem.Action = "fail"
			resultItem.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, resultItem)
			continue
		}

		merged := mergeDatatypePayload(current, map[string]any{"values": values})
		if reflect.DeepEqual(current, merged) {
			resultItem.Action = "skip"
			resultItem.Message = "already up to date"
			result.Skipped++
			result.Items = append(result.Items, resultItem)
			continue
		}

		if _, err := client.Put(ctx, fmt.Sprintf("/document/%s", id), merged, api.RequestOptions{DryRun: opts.DryRun, SkipValidation: true}); err != nil {
			resultItem.Action = "fail"
			resultItem.Error = err.Error()
			result.Failed++
			result.Items = append(result.Items, resultItem)
			continue
		}

		resultItem.Action = "update"
		if opts.DryRun {
			resultItem.Message = "validated"
		} else {
			resultItem.Message = "updated"
		}
		result.Updated++
		result.Items = append(result.Items, resultItem)
	}

	return result, nil
}
