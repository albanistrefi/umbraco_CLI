package skills

import (
	"fmt"
	"strings"
)

// Recipe defines a multi-step workflow skill.
type Recipe struct {
	Name        string
	Title       string
	Description string
	Services    []string
	Steps       []string
	Tips        []string
}

var recipes = []Recipe{
	{
		Name:        "find-and-update-document",
		Title:       "Find and Update a Document by Path",
		Description: "Walk the content tree to find a document, update a property, and publish.",
		Services:    []string{"umbraco-tree", "umbraco-document"},
		Steps: []string{
			`umbraco tree walk "Home/Partners/Partner List" --output json`,
			`umbraco document get <id-from-step-1> --fields "id,name,values" --output json`,
			`umbraco document update <id> --property skills --value "C#;Go" --dry-run --output json`,
			`umbraco document update <id> --property skills --value "C#;Go" --output json`,
			`umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --dry-run --output json`,
			`umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --output json`,
		},
		Tips: []string{
			"Always dry-run mutations before executing.",
			"Use --fields on reads to keep responses small.",
			"Reuse IDs returned by API responses; never construct them manually.",
		},
	},
	{
		Name:        "bulk-update-from-csv",
		Title:       "Bulk Update Documents from CSV",
		Description: "Use csv-update to batch-modify a property across multiple documents.",
		Services:    []string{"umbraco-document"},
		Steps: []string{
			`umbraco document csv-update --file partners.csv --property skills --dry-run --output json`,
			`umbraco document csv-update --file partners.csv --property skills --output json`,
		},
		Tips: []string{
			"The CSV must have an `id` column with document UUIDs and a column matching the --property alias.",
			"Dry-run output shows what would change without modifying anything.",
		},
	},
	{
		Name:        "discover-and-modify-datatype",
		Title:       "Discover and Modify a Data Type",
		Description: "Search for a data type, inspect it, and update its configuration.",
		Services:    []string{"umbraco-datatype"},
		Steps: []string{
			`umbraco datatype search --query "rich text" --output json`,
			`umbraco datatype get <id> --output json`,
			`umbraco datatype extensions <id> --output json`,
			`umbraco datatype add-extension <id> My.Custom.Extension --dry-run --output json`,
			`umbraco datatype add-extension <id> My.Custom.Extension --output json`,
		},
		Tips: []string{
			"Use `datatype extensions` to see the current extension list before modifying.",
			"add-extension and remove-extension handle fetch-merge-write automatically.",
		},
	},
	{
		Name:        "search-and-publish-documents",
		Title:       "Search and Publish Documents",
		Description: "Search for documents matching a query and publish them.",
		Services:    []string{"umbraco-document"},
		Steps: []string{
			`umbraco document search --query "draft" --output json`,
			`umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --dry-run --output json`,
			`umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --output json`,
		},
		Tips: []string{
			"Use --under <parent-id> to scope search to a subtree.",
			"Publish each document individually after reviewing the search results.",
		},
	},
}

func renderRecipeSkill(recipe Recipe, version string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`---
name: recipe-%s
description: "%s"
metadata:
  version: %s
  requires:
    bins:
      - umbraco
    skills:
`, recipe.Name, escapeYAML(recipe.Description), version))

	for _, svc := range recipe.Services {
		b.WriteString(fmt.Sprintf("      - %s\n", svc))
	}
	b.WriteString("---\n\n")

	b.WriteString(fmt.Sprintf("# %s\n\n", recipe.Title))

	b.WriteString(fmt.Sprintf("> **PREREQUISITE:** Load the following skills: %s\n\n",
		strings.Join(recipe.Services, ", ")))

	b.WriteString(fmt.Sprintf("%s\n\n", recipe.Description))

	b.WriteString("## Steps\n\n")
	for i, step := range recipe.Steps {
		b.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, step))
	}
	b.WriteString("\n")

	if len(recipe.Tips) > 0 {
		b.WriteString("## Tips\n\n")
		for _, tip := range recipe.Tips {
			b.WriteString(fmt.Sprintf("- %s\n", tip))
		}
		b.WriteString("\n")
	}

	return b.String()
}
