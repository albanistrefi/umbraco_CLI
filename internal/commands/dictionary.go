package commands

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/validate"
)

type dictionaryEntityReference struct {
	ID string `json:"id"`
}

type dictionaryTranslation struct {
	ISOCode     string `json:"isoCode"`
	Translation string `json:"translation"`
}

type dictionaryOverview struct {
	ID                 string                     `json:"id"`
	Name               string                     `json:"name"`
	Parent             *dictionaryEntityReference `json:"parent"`
	TranslatedISOCodes []string                   `json:"translatedIsoCodes"`
}

type dictionaryListResponse struct {
	Total int                  `json:"total"`
	Items []dictionaryOverview `json:"items"`
}

type dictionaryItem struct {
	Name         string                     `json:"name"`
	Parent       *dictionaryEntityReference `json:"parent"`
	Translations []dictionaryTranslation    `json:"translations"`
	ID           string                     `json:"id"`
}

type dictionaryCreateRequest struct {
	Name         string                     `json:"name"`
	Parent       *dictionaryEntityReference `json:"parent"`
	Translations []dictionaryTranslation    `json:"translations"`
	ID           string                     `json:"id"`
}

type dictionaryUpdateRequest struct {
	Name         string                     `json:"name"`
	Parent       *dictionaryEntityReference `json:"parent"`
	Translations []dictionaryTranslation    `json:"translations"`
}

type dictionaryExportItem struct {
	Key          string            `json:"key"`
	Translations map[string]string `json:"translations"`
}

func RegisterDictionary(root *cobra.Command, deps Dependencies) {
	dictionary := &cobra.Command{
		Use:   "dictionary",
		Short: "Dictionary item and translation key operations",
	}

	dictionary.AddCommand(dictionaryList(deps))
	dictionary.AddCommand(dictionaryGet(deps))
	dictionary.AddCommand(dictionaryCreate(deps))
	dictionary.AddCommand(dictionaryDelete(deps))
	dictionary.AddCommand(dictionaryExport(deps))
	dictionary.AddCommand(dictionaryImport(deps))

	root.AddCommand(dictionary)
}

func dictionaryList(deps Dependencies) *cobra.Command {
	var filter string
	var skip int
	var take int
	var triage readTriageOptions

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List dictionary items",
		RunE: func(cmd *cobra.Command, args []string) error {
			if skip < 0 {
				return fmt.Errorf("--skip must be zero or greater")
			}
			if take <= 0 {
				return fmt.Errorf("--take must be greater than zero")
			}

			params := map[string]any{
				"skip": skip,
				"take": take,
			}
			if strings.TrimSpace(filter) != "" {
				params["filter"] = filter
			}

			result, err := deps.Client.Get(context.Background(), "/dictionary", api.RequestOptions{Params: params})
			if err != nil {
				return err
			}

			return printResult(cmd, deps, applyReadTriage(result, triage))
		},
	}

	cmd.Flags().StringVar(&filter, "filter", "", "Filter dictionary items by key name")
	cmd.Flags().IntVar(&skip, "skip", 0, "Pagination offset")
	cmd.Flags().IntVar(&take, "take", 100, "Pagination page size")
	addReadTriageFlags(cmd, &triage)
	return cmd
}

func dictionaryGet(deps Dependencies) *cobra.Command {
	var key string

	cmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get a dictionary item by ID or key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := dictionaryResolveID(context.Background(), deps.Client, args, key)
			if err != nil {
				return err
			}

			result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/dictionary/%s", id), api.RequestOptions{})
			if err != nil {
				return err
			}

			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Dictionary key name")
	return cmd
}

func dictionaryCreate(deps Dependencies) *cobra.Command {
	var key string
	var parentID string
	var jsonPayload string
	var translations []string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a dictionary item",
		RunE: func(cmd *cobra.Command, args []string) error {
			var body any

			if strings.TrimSpace(jsonPayload) != "" {
				payload, err := parsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = payload
			} else {
				if err := requireValue("--key", key); err != nil {
					return err
				}
				if err := validate.String(key); err != nil {
					return err
				}

				parent, err := dictionaryParentReference(parentID)
				if err != nil {
					return err
				}

				parsedTranslations, err := parseDictionaryTranslations(translations)
				if err != nil {
					return err
				}
				if len(parsedTranslations) == 0 {
					return fmt.Errorf("dictionary create requires at least one --translation or --json payload")
				}

				body = dictionaryCreateRequest{
					Name:         key,
					Parent:       parent,
					Translations: parsedTranslations,
					ID:           deterministicDictionaryUUID(key),
				}
			}

			result, err := deps.Client.Post(context.Background(), "/dictionary", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Dictionary key name")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "Optional parent dictionary item ID")
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Full JSON payload")
	cmd.Flags().StringArrayVar(&translations, "translation", nil, "Translation in isoCode=value format; repeat for multiple locales")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func dictionaryDelete(deps Dependencies) *cobra.Command {
	var key string
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a dictionary item by ID or key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force && !dryRun {
				return fmt.Errorf("dictionary delete requires --force or --dry-run")
			}

			id, err := dictionaryResolveID(context.Background(), deps.Client, args, key)
			if err != nil {
				return err
			}

			result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/dictionary/%s", id), api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}

			return printResult(cmd, deps, result)
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Dictionary key name")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func dictionaryExport(deps Dependencies) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all dictionary items to JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			items, err := listAllDictionaryItems(ctx, deps.Client, "", 9999)
			if err != nil {
				return err
			}

			exported := make([]dictionaryExportItem, 0, len(items))
			for _, item := range items {
				detail, err := getDictionaryByID(ctx, deps.Client, item.ID)
				if err != nil {
					return err
				}

				exported = append(exported, dictionaryExportItem{
					Key:          detail.Name,
					Translations: dictionaryTranslationsToMap(detail.Translations),
				})
			}

			sort.Slice(exported, func(i, j int) bool {
				return exported[i].Key < exported[j].Key
			})

			if strings.TrimSpace(file) == "" {
				return printResult(cmd, deps, exported)
			}

			payload, err := json.MarshalIndent(exported, "", "  ")
			if err != nil {
				return err
			}
			payload = append(payload, '\n')

			if err := os.WriteFile(file, payload, 0o644); err != nil {
				return err
			}

			return printResult(cmd, deps, map[string]any{
				"file":  file,
				"count": len(exported),
			})
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Write exported JSON to a file instead of stdout")
	return cmd
}

func dictionaryResolveID(ctx context.Context, client *api.Client, args []string, key string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(key) != "" {
		return "", fmt.Errorf("provide either an ID argument or --key, not both")
	}
	if len(args) > 0 {
		return args[0], nil
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("provide a dictionary item ID or --key")
	}

	item, err := findDictionaryByKey(ctx, client, key)
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

func listDictionaryPage(ctx context.Context, client *api.Client, filter string, skip int, take int) (dictionaryListResponse, error) {
	params := map[string]any{
		"skip": skip,
		"take": take,
	}
	if strings.TrimSpace(filter) != "" {
		params["filter"] = filter
	}

	result, err := client.Get(ctx, "/dictionary", api.RequestOptions{Params: params})
	if err != nil {
		return dictionaryListResponse{}, err
	}

	return decodeResult[dictionaryListResponse](result)
}

func listAllDictionaryItems(ctx context.Context, client *api.Client, filter string, pageSize int) ([]dictionaryOverview, error) {
	if pageSize <= 0 {
		pageSize = 100
	}

	all := make([]dictionaryOverview, 0)
	skip := 0

	for {
		page, err := listDictionaryPage(ctx, client, filter, skip, pageSize)
		if err != nil {
			return nil, err
		}

		all = append(all, page.Items...)
		skip += len(page.Items)

		if len(page.Items) == 0 || skip >= page.Total {
			break
		}
	}

	return all, nil
}

func findDictionaryByKey(ctx context.Context, client *api.Client, key string) (dictionaryOverview, error) {
	if err := validate.String(key); err != nil {
		return dictionaryOverview{}, err
	}

	items, err := listAllDictionaryItems(ctx, client, key, 100)
	if err != nil {
		return dictionaryOverview{}, err
	}

	for _, item := range items {
		if item.Name == key {
			return item, nil
		}
	}

	return dictionaryOverview{}, fmt.Errorf("dictionary item not found: %s", key)
}

func getDictionaryByID(ctx context.Context, client *api.Client, id string) (dictionaryItem, error) {
	result, err := client.Get(ctx, fmt.Sprintf("/dictionary/%s", id), api.RequestOptions{})
	if err != nil {
		return dictionaryItem{}, err
	}

	return decodeResult[dictionaryItem](result)
}

func dictionaryParentReference(parentID string) (*dictionaryEntityReference, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil, nil
	}
	if err := validate.ResourceID(parentID); err != nil {
		return nil, err
	}
	return &dictionaryEntityReference{ID: parentID}, nil
}

func parseDictionaryTranslations(raw []string) ([]dictionaryTranslation, error) {
	seen := make(map[string]struct{}, len(raw))
	parsed := make([]dictionaryTranslation, 0, len(raw))

	for _, entry := range raw {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --translation value %q, expected isoCode=value", entry)
		}

		isoCode := strings.TrimSpace(parts[0])
		if isoCode == "" {
			return nil, fmt.Errorf("translation isoCode cannot be empty")
		}
		if err := validate.String(isoCode); err != nil {
			return nil, err
		}
		if err := validate.String(parts[1]); err != nil {
			return nil, err
		}

		if _, exists := seen[isoCode]; exists {
			return nil, fmt.Errorf("duplicate translation isoCode: %s", isoCode)
		}
		seen[isoCode] = struct{}{}

		parsed = append(parsed, dictionaryTranslation{
			ISOCode:     isoCode,
			Translation: parts[1],
		})
	}

	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].ISOCode < parsed[j].ISOCode
	})

	return parsed, nil
}

func dictionaryTranslationsFromMap(translations map[string]string) []dictionaryTranslation {
	items := make([]dictionaryTranslation, 0, len(translations))
	for isoCode, translation := range translations {
		items = append(items, dictionaryTranslation{
			ISOCode:     isoCode,
			Translation: translation,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ISOCode < items[j].ISOCode
	})

	return items
}

func dictionaryTranslationsToMap(translations []dictionaryTranslation) map[string]string {
	result := make(map[string]string, len(translations))
	for _, translation := range translations {
		result[translation.ISOCode] = translation.Translation
	}
	return result
}

func deterministicDictionaryUUID(key string) string {
	sum := sha1.Sum([]byte("umbraco-cli:dictionary:" + key))
	bytes := sum[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}
