// Command gen generates openapi_generated.go from the vendored Management
// API OpenAPI document (openapi.json). Refresh the document from a running
// instance with:
//
//	curl -sk https://localhost:44314/umbraco/swagger/management/swagger.json -o internal/schema/gen/openapi.json
//
// then run `go generate ./internal/schema`. CI fails when the generated
// file drifts from the vendored document.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

const apiPrefix = "/umbraco/management/api/v1"

type document struct {
	Paths      map[string]map[string]operation `json:"paths"`
	Components struct {
		Schemas map[string]jsonSchema `json:"schemas"`
	} `json:"components"`
}

type operation struct {
	Parameters  []parameter         `json:"parameters"`
	RequestBody *requestBody        `json:"requestBody"`
	Responses   map[string]struct{} `json:"-"`
}

type parameter struct {
	Name     string     `json:"name"`
	In       string     `json:"in"`
	Required bool       `json:"required"`
	Schema   jsonSchema `json:"schema"`
}

type requestBody struct {
	Content map[string]struct {
		Schema jsonSchema `json:"schema"`
	} `json:"content"`
}

type jsonSchema struct {
	Ref         string                `json:"$ref"`
	OneOf       []jsonSchema          `json:"oneOf"`
	Type        string                `json:"type"`
	Format      string                `json:"format"`
	Description string                `json:"description"`
	Nullable    bool                  `json:"nullable"`
	Required    []string              `json:"required"`
	Properties  map[string]jsonSchema `json:"properties"`
	Items       *jsonSchema           `json:"items"`
	Enum        []any                 `json:"enum"`
}

type generator struct {
	schemas map[string]jsonSchema
}

// resolve follows $ref and single-element oneOf wrappers to the concrete
// schema, returning the last referenced model name for labelling.
func (g *generator) resolve(s jsonSchema) (jsonSchema, string) {
	name := ""
	for i := 0; i < 8; i++ {
		switch {
		case s.Ref != "":
			name = strings.TrimPrefix(s.Ref, "#/components/schemas/")
			s = g.schemas[name]
		case len(s.OneOf) == 1:
			s = s.OneOf[0]
		default:
			return s, name
		}
	}
	return s, name
}

// typeString renders a schema as the short human/agent-readable type label
// used in property maps.
func (g *generator) typeString(s jsonSchema) string {
	resolved, name := g.resolve(s)
	switch resolved.Type {
	case "array":
		if resolved.Items == nil {
			return "array"
		}
		return "array<" + g.typeString(*resolved.Items) + ">"
	case "string":
		if len(resolved.Enum) > 0 {
			values := make([]string, 0, len(resolved.Enum))
			for _, v := range resolved.Enum {
				values = append(values, fmt.Sprint(v))
			}
			return "enum: " + strings.Join(values, "|")
		}
		if resolved.Format != "" {
			return "string (" + resolved.Format + ")"
		}
		return "string"
	case "boolean", "number", "integer":
		return resolved.Type
	case "object", "":
		if name != "" {
			if _, hasID := resolved.Properties["id"]; hasID && len(resolved.Properties) == 1 {
				return "{id} reference"
			}
			return "object (" + name + ")"
		}
		return "object"
	default:
		return resolved.Type
	}
}

func (g *generator) paramSchema(p parameter) (string, string, string) {
	resolved, _ := g.resolve(p.Schema)
	typ := resolved.Type
	format := resolved.Format
	description := resolved.Description
	if typ == "array" && resolved.Items != nil {
		item, _ := g.resolve(*resolved.Items)
		format = item.Format
		if description == "" {
			description = "Repeat the " + p.Name + " query parameter for each value"
		}
	}
	if typ == "integer" {
		typ = "number"
	}
	return typ, format, description
}

func (g *generator) requestBodySchema(rb *requestBody) (string, []string, map[string]string, string) {
	if rb == nil {
		return "", nil, nil, ""
	}
	if content, ok := rb.Content["application/json"]; ok {
		resolved, name := g.resolve(content.Schema)
		if resolved.Type == "array" {
			item := "object"
			if resolved.Items != nil {
				item = g.typeString(*resolved.Items)
			}
			return "array", nil, nil, "Array of " + item + " entries"
		}
		props := make(map[string]string, len(resolved.Properties))
		for key, prop := range resolved.Properties {
			props[key] = g.typeString(prop)
		}
		description := ""
		if name != "" {
			description = name
		}
		return "object", resolved.Required, props, description
	}
	if _, ok := rb.Content["multipart/form-data"]; ok {
		return "object", nil, nil, "multipart/form-data upload"
	}
	return "object", nil, nil, "Raw payload accepted by the endpoint"
}

func quote(s string) string {
	return fmt.Sprintf("%q", s)
}

func main() {
	payload, err := os.ReadFile("gen/openapi.json")
	if err != nil {
		log.Fatalf("read openapi document: %v", err)
	}
	var doc document
	if err := json.Unmarshal(payload, &doc); err != nil {
		log.Fatalf("parse openapi document: %v", err)
	}
	g := &generator{schemas: doc.Components.Schemas}

	type entry struct {
		key  string
		code string
	}
	entries := make([]entry, 0, len(doc.Paths)*2)

	for fullPath, ops := range doc.Paths {
		relative := strings.TrimPrefix(fullPath, apiPrefix)
		if relative == fullPath {
			continue // outside the management mount
		}
		for method, op := range ops {
			upper := strings.ToUpper(method)
			if upper != "GET" && upper != "POST" && upper != "PUT" && upper != "DELETE" && upper != "PATCH" {
				continue
			}

			var b strings.Builder
			fmt.Fprintf(&b, "{Method: %s, Path: %s", quote(upper), quote(relative))

			pathParams := make([]parameter, 0)
			queryParams := make([]parameter, 0)
			for _, p := range op.Parameters {
				switch p.In {
				case "path":
					pathParams = append(pathParams, p)
				case "query":
					queryParams = append(queryParams, p)
				}
			}
			writeParams := func(label string, params []parameter) {
				if len(params) == 0 {
					return
				}
				sort.Slice(params, func(i, j int) bool { return params[i].Name < params[j].Name })
				fmt.Fprintf(&b, ", %s: map[string]ParamSchema{", label)
				for i, p := range params {
					if i > 0 {
						b.WriteString(", ")
					}
					typ, format, description := g.paramSchema(p)
					fmt.Fprintf(&b, "%s: {Type: %s", quote(p.Name), quote(typ))
					if format != "" {
						fmt.Fprintf(&b, ", Format: %s", quote(format))
					}
					if p.Required {
						b.WriteString(", Required: true")
					}
					if description != "" {
						fmt.Fprintf(&b, ", Description: %s", quote(description))
					}
					b.WriteString("}")
				}
				b.WriteString("}")
			}
			writeParams("PathParams", pathParams)
			writeParams("QueryParams", queryParams)

			if bodyType, required, props, description := g.requestBodySchema(op.RequestBody); bodyType != "" {
				fmt.Fprintf(&b, ", RequestBody: &ObjectSchema{Type: %s", quote(bodyType))
				if len(required) > 0 {
					sort.Strings(required)
					quoted := make([]string, len(required))
					for i, r := range required {
						quoted[i] = quote(r)
					}
					fmt.Fprintf(&b, ", Required: []string{%s}", strings.Join(quoted, ", "))
				}
				if len(props) > 0 {
					keys := make([]string, 0, len(props))
					for k := range props {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					b.WriteString(", Properties: map[string]any{")
					for i, k := range keys {
						if i > 0 {
							b.WriteString(", ")
						}
						fmt.Fprintf(&b, "%s: %s", quote(k), quote(props[k]))
					}
					b.WriteString("}")
				}
				if description != "" {
					fmt.Fprintf(&b, ", Description: %s", quote(description))
				}
				b.WriteString("}")
			}
			b.WriteString("}")

			entries = append(entries, entry{key: upper + " " + relative, code: b.String()})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	var out strings.Builder
	out.WriteString("// Code generated by go generate ./internal/schema; DO NOT EDIT.\n")
	out.WriteString("// Source: gen/openapi.json (Umbraco Management API OpenAPI document).\n\n")
	out.WriteString("package schema\n\n")
	out.WriteString("// openAPIOperations maps \"METHOD /path\" (relative to the API mount) to\n")
	out.WriteString("// the operation detail published in the Management API OpenAPI document.\n")
	out.WriteString("var openAPIOperations = map[string]rawSchema{\n")
	for _, e := range entries {
		fmt.Fprintf(&out, "\t%s: %s,\n", quote(e.key), e.code)
	}
	out.WriteString("}\n")

	if err := os.WriteFile("openapi_generated.go", []byte(out.String()), 0o644); err != nil {
		log.Fatalf("write generated file: %v", err)
	}
	fmt.Printf("generated %d operations\n", len(entries))
}
