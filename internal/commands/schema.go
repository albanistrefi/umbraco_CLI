package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/schema"
)

func RegisterSchema(root *cobra.Command, deps Dependencies) {
	var list bool
	var printTemplate bool
	schemaCommand := &cobra.Command{
		Use:   "schema [endpoint]",
		Short: "Introspect API endpoint schemas",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				return printResult(cmd, deps, map[string]any{"endpoints": visibleSchemaEndpoints()})
			}

			key := args[0]
			if !automateEnabled() && isAutomateSchemaKey(key) {
				return fmt.Errorf("unknown endpoint or collection: %s. Run 'umbraco schema --list'", key)
			}

			if printTemplate {
				template, ok := schema.Templates[key]
				if !ok {
					return fmt.Errorf("no JSON template for endpoint: %s", key)
				}
				return printResult(cmd, deps, template)
			}

			if endpointSchema, ok := schema.Schemas[key]; ok {
				return printResult(cmd, deps, endpointSchema)
			}

			prefix := key + "."
			matches := make([]string, 0)
			for _, endpoint := range visibleSchemaEndpoints() {
				if strings.HasPrefix(endpoint, prefix) {
					matches = append(matches, endpoint)
				}
			}
			if len(matches) > 0 {
				return printResult(cmd, deps, map[string]any{"collection": key, "endpoints": matches})
			}

			return fmt.Errorf("unknown endpoint or collection: %s. Run 'umbraco schema --list'", key)
		},
	}
	schemaCommand.Flags().BoolVar(&list, "list", false, "List available endpoints")
	schemaCommand.Flags().BoolVar(&printTemplate, "template", false, "Print a JSON payload template for the endpoint")
	schemaCommand.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List schema endpoints",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printResult(cmd, deps, map[string]any{"endpoints": visibleSchemaEndpoints()})
		},
	})
	root.AddCommand(schemaCommand)
}

// visibleSchemaEndpoints hides Automate entries while the command group is
// feature-gated, so schema discovery matches the registered command tree.
func visibleSchemaEndpoints() []string {
	if automateEnabled() {
		return schema.Endpoints
	}
	endpoints := make([]string, 0, len(schema.Endpoints))
	for _, endpoint := range schema.Endpoints {
		if !isAutomateSchemaKey(endpoint) {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}

func isAutomateSchemaKey(key string) bool {
	return key == "automate" || strings.HasPrefix(key, "automate.")
}
