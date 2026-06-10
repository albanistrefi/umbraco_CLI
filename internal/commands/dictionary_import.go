package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
	"umbraco-cli/internal/validate"
)

type dictionaryImportFileItem struct {
	Key          string            `json:"key"`
	Translations map[string]string `json:"translations"`
}

type dictionaryImportOptions struct {
	File           string
	SkipExisting   bool
	UpdateExisting bool
	DryRun         bool
	BatchSize      int
}

type dictionaryImportItemResult struct {
	Key     string `json:"key"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type dictionaryImportResult struct {
	File    string                       `json:"file"`
	DryRun  bool                         `json:"dryRun"`
	Created int                          `json:"created"`
	Updated int                          `json:"updated"`
	Skipped int                          `json:"skipped"`
	Failed  int                          `json:"failed"`
	Items   []dictionaryImportItemResult `json:"items"`
}

type dictionaryImportPrinter struct {
	cmd     *cobra.Command
	enabled bool
	dryRun  bool
	mu      sync.Mutex
}

func dictionaryImport(deps Dependencies) *cobra.Command {
	var file string
	var skipExisting bool
	var updateExisting bool
	var dryRun bool
	var batchSize int

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import dictionary items from JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--file", file); err != nil {
				return err
			}
			if batchSize < 1 || batchSize > 10 {
				return fmt.Errorf("--batch-size must be between 1 and 10")
			}
			if updateExisting && cmd.Flags().Changed("skip-existing") && skipExisting {
				return fmt.Errorf("cannot combine --update-existing with --skip-existing=true")
			}
			if updateExisting {
				skipExisting = false
			}
			if !skipExisting && !updateExisting {
				return fmt.Errorf("existing items handling is ambiguous; use the default skip behavior or set --update-existing")
			}

			format, err := resolveOutputFormat(deps)
			if err != nil {
				return err
			}

			result, err := executeDictionaryImport(
				cmd.Context(),
				cmd,
				deps,
				dictionaryImportOptions{
					File:           file,
					SkipExisting:   skipExisting,
					UpdateExisting: updateExisting,
					DryRun:         dryRun,
					BatchSize:      batchSize,
				},
				format == config.OutputJSON,
			)
			if err != nil {
				return err
			}

			if format == config.OutputJSON {
				return printResult(cmd, deps, result)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Path to the dictionary import JSON file")
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", true, "Skip items that already exist")
	cmd.Flags().BoolVar(&updateExisting, "update-existing", false, "Merge translations into existing items")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Plan the import without writing changes")
	cmd.Flags().IntVar(&batchSize, "batch-size", 5, "Maximum concurrent create or update requests (1-10)")
	return cmd
}

func executeDictionaryImport(ctx context.Context, cmd *cobra.Command, deps Dependencies, opts dictionaryImportOptions, structuredOutput bool) (dictionaryImportResult, error) {
	items, err := loadDictionaryImportItems(opts.File)
	if err != nil {
		return dictionaryImportResult{}, err
	}

	existingItems, err := listAllDictionaryItems(ctx, deps.Client, "", 9999)
	if err != nil {
		return dictionaryImportResult{}, err
	}

	existingByKey := make(map[string]dictionaryOverview, len(existingItems))
	for _, item := range existingItems {
		existingByKey[item.Name] = item
	}

	results := make([]dictionaryImportItemResult, len(items))
	sem := make(chan struct{}, opts.BatchSize)
	var wg sync.WaitGroup
	printer := dictionaryImportPrinter{cmd: cmd, enabled: !structuredOutput, dryRun: opts.DryRun}

	for index, item := range items {
		if existing, found := existingByKey[item.Key]; found && opts.SkipExisting {
			results[index] = dictionaryImportItemResult{
				Key:     item.Key,
				Action:  "skip",
				Message: fmt.Sprintf("already exists (%s)", existing.ID),
			}
			printer.PrintItem(results[index])
			continue
		}

		item := item
		existing, found := existingByKey[item.Key]

		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() {
				<-sem
			}()

			var existingPtr *dictionaryOverview
			if found {
				existingCopy := existing
				existingPtr = &existingCopy
			}

			result := processDictionaryImportItem(ctx, deps.Client, item, existingPtr, opts)
			results[index] = result
			printer.PrintItem(result)
		}(index)
	}

	wg.Wait()

	result := dictionaryImportResult{
		File:   opts.File,
		DryRun: opts.DryRun,
		Items:  results,
	}
	for _, item := range results {
		switch item.Action {
		case "create":
			result.Created++
		case "update":
			result.Updated++
		case "skip":
			result.Skipped++
		case "fail":
			result.Failed++
		}
	}

	printer.PrintSummary(result)
	return result, nil
}

func processDictionaryImportItem(ctx context.Context, client *api.Client, item dictionaryImportFileItem, existing *dictionaryOverview, opts dictionaryImportOptions) dictionaryImportItemResult {
	if existing == nil {
		payload := dictionaryCreateRequest{
			Name:         item.Key,
			Parent:       nil,
			Translations: dictionaryTranslationsFromMap(item.Translations),
			ID:           deterministicDictionaryUUID(item.Key),
		}

		if _, err := client.Post(ctx, "/dictionary", payload, api.RequestOptions{DryRun: opts.DryRun}); err != nil {
			return dictionaryImportItemResult{
				Key:    item.Key,
				Action: "fail",
				Error:  err.Error(),
			}
		}

		return dictionaryImportItemResult{
			Key:     item.Key,
			Action:  "create",
			Message: "created",
		}
	}

	current, err := getDictionaryByID(ctx, client, existing.ID)
	if err != nil {
		return dictionaryImportItemResult{
			Key:    item.Key,
			Action: "fail",
			Error:  err.Error(),
		}
	}

	if !opts.UpdateExisting {
		return dictionaryImportItemResult{
			Key:     item.Key,
			Action:  "skip",
			Message: fmt.Sprintf("already exists (%s)", existing.ID),
		}
	}

	mergedTranslations, changed := mergeDictionaryTranslations(current.Translations, item.Translations)
	if !changed {
		return dictionaryImportItemResult{
			Key:     item.Key,
			Action:  "skip",
			Message: "already up to date",
		}
	}

	payload := dictionaryUpdateRequest{
		Name:         current.Name,
		Parent:       current.Parent,
		Translations: mergedTranslations,
	}
	if _, err := client.Put(ctx, api.JoinPath("/dictionary/%s", existing.ID), payload, api.RequestOptions{DryRun: opts.DryRun}); err != nil {
		return dictionaryImportItemResult{
			Key:    item.Key,
			Action: "fail",
			Error:  err.Error(),
		}
	}

	return dictionaryImportItemResult{
		Key:     item.Key,
		Action:  "update",
		Message: "updated",
	}
}

func loadDictionaryImportItems(path string) ([]dictionaryImportFileItem, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []dictionaryImportFileItem
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("invalid dictionary import JSON: %w", err)
	}

	mergedByKey := make(map[string]dictionaryImportFileItem, len(items))
	keyOrder := make([]string, 0, len(items))

	for index, item := range items {
		if strings.TrimSpace(item.Key) == "" {
			return nil, fmt.Errorf("import item %d is missing a key", index)
		}
		if item.Key != strings.TrimSpace(item.Key) {
			return nil, fmt.Errorf("import item %q has leading or trailing whitespace in key", item.Key)
		}
		if err := validate.String(item.Key); err != nil {
			return nil, err
		}

		if len(item.Translations) == 0 {
			return nil, fmt.Errorf("import item %q must contain at least one translation", item.Key)
		}

		mergedItem, exists := mergedByKey[item.Key]
		if !exists {
			mergedItem = dictionaryImportFileItem{
				Key:          item.Key,
				Translations: make(map[string]string, len(item.Translations)),
			}
			keyOrder = append(keyOrder, item.Key)
		}

		for isoCode, translation := range item.Translations {
			if strings.TrimSpace(isoCode) == "" {
				return nil, fmt.Errorf("import item %q contains an empty isoCode", item.Key)
			}
			if err := validate.String(isoCode); err != nil {
				return nil, err
			}

			if existingTranslation, alreadySet := mergedItem.Translations[isoCode]; alreadySet && existingTranslation != translation {
				return nil, fmt.Errorf("conflicting translations for key %q and isoCode %q", item.Key, isoCode)
			}
			mergedItem.Translations[isoCode] = translation
		}

		mergedByKey[item.Key] = mergedItem
	}

	mergedItems := make([]dictionaryImportFileItem, 0, len(keyOrder))
	for _, key := range keyOrder {
		mergedItems = append(mergedItems, mergedByKey[key])
	}

	return mergedItems, nil
}

func mergeDictionaryTranslations(existing []dictionaryTranslation, incoming map[string]string) ([]dictionaryTranslation, bool) {
	merged := dictionaryTranslationsToMap(existing)
	changed := false

	for isoCode, translation := range incoming {
		current, exists := merged[isoCode]
		if !exists || current != translation {
			changed = true
		}
		merged[isoCode] = translation
	}

	return dictionaryTranslationsFromMap(merged), changed
}

func (p *dictionaryImportPrinter) PrintItem(item dictionaryImportItemResult) {
	if !p.enabled {
		return
	}

	line := p.lineForItem(item)

	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.cmd.OutOrStdout(), line)
}

func (p *dictionaryImportPrinter) PrintSummary(result dictionaryImportResult) {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.dryRun {
		fmt.Fprintf(
			p.cmd.OutOrStdout(),
			"Dry run summary: Create: %d, Update: %d, Skip: %d, Failed: %d\n",
			result.Created,
			result.Updated,
			result.Skipped,
			result.Failed,
		)
		return
	}

	fmt.Fprintf(
		p.cmd.OutOrStdout(),
		"Created: %d, Updated: %d, Skipped: %d, Failed: %d\n",
		result.Created,
		result.Updated,
		result.Skipped,
		result.Failed,
	)
}

func (p *dictionaryImportPrinter) lineForItem(item dictionaryImportItemResult) string {
	switch item.Action {
	case "create":
		if p.dryRun {
			return fmt.Sprintf("Would create: %s", item.Key)
		}
		return fmt.Sprintf("Creating %s... OK", item.Key)
	case "update":
		if p.dryRun {
			return fmt.Sprintf("Would update: %s", item.Key)
		}
		return fmt.Sprintf("Updating %s... OK", item.Key)
	case "skip":
		if p.dryRun {
			return fmt.Sprintf("Would skip: %s (%s)", item.Key, item.Message)
		}
		return fmt.Sprintf("Skipping %s (%s)", item.Key, item.Message)
	default:
		return fmt.Sprintf("Failed: %s (%s)", item.Key, item.Error)
	}
}
