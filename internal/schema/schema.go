package schema

import "sort"

//go:generate go run ./gen

type ParamSchema struct {
	Type        string `json:"type"`
	Format      string `json:"format,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

type ObjectSchema struct {
	Type        string         `json:"type"`
	Required    []string       `json:"required,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Items       any            `json:"items,omitempty"`
	Description string         `json:"description,omitempty"`
}

type Schema struct {
	Endpoint    string                 `json:"endpoint"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	PathParams  map[string]ParamSchema `json:"pathParams,omitempty"`
	QueryParams map[string]ParamSchema `json:"queryParams,omitempty"`
	RequestBody *ObjectSchema          `json:"requestBody,omitempty"`
	Response    *ObjectSchema          `json:"response,omitempty"`
}

type rawSchema struct {
	Method      string
	Path        string
	PathParams  map[string]ParamSchema
	QueryParams map[string]ParamSchema
	RequestBody *ObjectSchema
	Response    *ObjectSchema
}

// endpointBinding ties a CLI command to a Management API operation. The
// parameter and request-body detail comes from openapi_generated.go (built
// from the vendored OpenAPI document via go generate), so it cannot drift
// from what the server actually accepts. Bindings only declare what is
// CLI-specific: which operation a command maps to, CLI-level query
// parameters layered on top, and response annotations.
type endpointBinding struct {
	Method string
	Path   string
	// ExtraQuery documents CLI-level parameters (e.g. fields) that are not
	// part of the Management API operation.
	ExtraQuery map[string]ParamSchema
	// Response annotates CLI-specific response handling.
	Response *ObjectSchema
	// Manual supplies the full schema for endpoints outside the vendored
	// document (CLI workflows, version-fallback paths). When set, no
	// OpenAPI lookup happens.
	Manual *rawSchema
}

var (
	fieldsQuery = ParamSchema{Type: "string", Description: "Comma-separated field list (applied client-side by the CLI)"}
	withFields  = map[string]ParamSchema{"fields": fieldsQuery}
)

var endpointBindings = map[string]endpointBinding{
	// document
	"document.get":                        {Method: "GET", Path: "/document/{id}", ExtraQuery: withFields},
	"document.root":                       {Method: "GET", Path: "/tree/document/root", ExtraQuery: withFields},
	"document.children":                   {Method: "GET", Path: "/tree/document/children", ExtraQuery: withFields},
	"document.ancestors":                  {Method: "GET", Path: "/tree/document/ancestors"},
	"document.search":                     {Method: "GET", Path: "/item/document/search"},
	"document.create":                     {Method: "POST", Path: "/document"},
	"document.update":                     {Method: "PUT", Path: "/document/{id}"},
	"document.update-properties":          {Method: "PUT", Path: "/document/{id}", Response: &ObjectSchema{Type: "object", Description: "CLI convenience wrapper that fetches, merges, and writes the full document payload"}},
	"document.publish":                    {Method: "PUT", Path: "/document/{id}/publish"},
	"document.unpublish":                  {Method: "PUT", Path: "/document/{id}/unpublish"},
	"document.publish-descendants":        {Method: "PUT", Path: "/document/{id}/publish-with-descendants", Response: &ObjectSchema{Type: "object", Description: "Asynchronous; carries a taskId for publish-descendants-result"}},
	"document.publish-descendants-result": {Method: "GET", Path: "/document/{id}/publish-with-descendants/result/{taskId}"},
	"document.sort":                       {Method: "PUT", Path: "/document/sort"},
	"document.audit-log":                  {Method: "GET", Path: "/document/{id}/audit-log"},
	"document.copy":                       {Method: "POST", Path: "/document/{id}/copy"},
	"document.move":                       {Method: "PUT", Path: "/document/{id}/move"},
	"document.delete":                     {Method: "DELETE", Path: "/document/{id}"},
	"document.trash":                      {Method: "PUT", Path: "/document/{id}/move-to-recycle-bin"},
	"document.restore":                    {Method: "PUT", Path: "/recycle-bin/document/{id}/restore"},
	"document.references":                 {Method: "GET", Path: "/document/{id}/referenced-by"},
	"document.referenced-descendants":     {Method: "GET", Path: "/document/{id}/referenced-descendants"},
	"document.are-referenced":             {Method: "GET", Path: "/document/are-referenced"},
	"document.version.list":               {Method: "GET", Path: "/document-version"},
	"document.version.get":                {Method: "GET", Path: "/document-version/{id}"},
	"document.version.rollback":           {Method: "POST", Path: "/document-version/{id}/rollback"},
	"document.version.prevent-cleanup":    {Method: "PUT", Path: "/document-version/{id}/prevent-cleanup"},
	"document.domains.get":                {Method: "GET", Path: "/document/{id}/domains"},
	"document.domains.set":                {Method: "PUT", Path: "/document/{id}/domains"},
	"document.public-access.get":          {Method: "GET", Path: "/document/{id}/public-access"},
	"document.public-access.set":          {Method: "POST", Path: "/document/{id}/public-access", Response: &ObjectSchema{Type: "object", Description: "CLI resolves create-vs-replace: POST when no rules exist, PUT otherwise"}},
	"document.public-access.remove":       {Method: "DELETE", Path: "/document/{id}/public-access"},

	// dictionary
	"dictionary.list":   {Method: "GET", Path: "/dictionary", ExtraQuery: withFields},
	"dictionary.get":    {Method: "GET", Path: "/dictionary/{id}"},
	"dictionary.create": {Method: "POST", Path: "/dictionary"},
	"dictionary.update": {Method: "PUT", Path: "/dictionary/{id}"},
	"dictionary.delete": {Method: "DELETE", Path: "/dictionary/{id}"},
	"dictionary.import": {Manual: &rawSchema{Method: "POST", Path: "/dictionary", RequestBody: &ObjectSchema{Type: "array", Description: "CLI workflow: reads a JSON file of {key, translations} items and creates/updates dictionary entries via POST /dictionary and PUT /dictionary/{id}"}}},
	"dictionary.export": {Manual: &rawSchema{Method: "GET", Path: "/dictionary", Response: &ObjectSchema{Type: "array", Description: "CLI workflow: aggregates list/get calls into [{key, translations}] JSON"}}},

	// media
	"media.get":                    {Method: "GET", Path: "/media/{id}", ExtraQuery: withFields},
	"media.root":                   {Method: "GET", Path: "/tree/media/root", ExtraQuery: withFields},
	"media.children":               {Method: "GET", Path: "/tree/media/children", ExtraQuery: withFields},
	"media.search":                 {Method: "GET", Path: "/item/media/search"},
	"media.urls":                   {Method: "GET", Path: "/media/urls"},
	"media.create":                 {Method: "POST", Path: "/media"},
	"media.create-folder":          {Method: "POST", Path: "/media", Response: &ObjectSchema{Type: "object", Description: "CLI workflow: resolves the Folder media type and creates a media item of that type"}},
	"media.upload":                 {Manual: &rawSchema{Method: "POST", Path: "/temporary-file", RequestBody: &ObjectSchema{Type: "object", Description: "CLI workflow: multipart temporary-file upload followed by media create"}}},
	"media.update":                 {Method: "PUT", Path: "/media/{id}"},
	"media.move":                   {Method: "PUT", Path: "/media/{id}/move"},
	"media.delete":                 {Method: "DELETE", Path: "/media/{id}"},
	"media.trash":                  {Method: "PUT", Path: "/media/{id}/move-to-recycle-bin"},
	"media.references":             {Method: "GET", Path: "/media/{id}/referenced-by"},
	"media.referenced-descendants": {Method: "GET", Path: "/media/{id}/referenced-descendants"},
	"media.are-referenced":         {Method: "GET", Path: "/media/are-referenced"},

	// doctype
	"doctype.get":      {Method: "GET", Path: "/document-type/{id}", ExtraQuery: withFields},
	"doctype.list":     {Method: "GET", Path: "/tree/document-type/root", ExtraQuery: withFields},
	"doctype.root":     {Method: "GET", Path: "/tree/document-type/root"},
	"doctype.children": {Method: "GET", Path: "/tree/document-type/children"},
	"doctype.search":   {Method: "GET", Path: "/item/document-type/search"},
	"doctype.create":   {Method: "POST", Path: "/document-type"},
	"doctype.update":   {Method: "PUT", Path: "/document-type/{id}"},
	"doctype.copy":     {Method: "POST", Path: "/document-type/{id}/copy"},
	"doctype.move":     {Method: "PUT", Path: "/document-type/{id}/move"},
	"doctype.delete":   {Method: "DELETE", Path: "/document-type/{id}"},

	// datatype
	"datatype.get":     {Method: "GET", Path: "/data-type/{id}", ExtraQuery: withFields},
	"datatype.list":    {Method: "GET", Path: "/filter/data-type"},
	"datatype.root":    {Method: "GET", Path: "/tree/data-type/root"},
	"datatype.search":  {Method: "GET", Path: "/item/data-type/search"},
	"datatype.is-used": {Method: "GET", Path: "/data-type/{id}/is-used"},
	"datatype.create":  {Method: "POST", Path: "/data-type"},
	"datatype.update":  {Method: "PUT", Path: "/data-type/{id}"},
	"datatype.delete":  {Method: "DELETE", Path: "/data-type/{id}"},

	// template
	"template.get":    {Method: "GET", Path: "/template/{id}", ExtraQuery: withFields},
	"template.root":   {Method: "GET", Path: "/tree/template/root"},
	"template.search": {Method: "GET", Path: "/item/template/search"},
	"template.create": {Method: "POST", Path: "/template"},
	"template.update": {Method: "PUT", Path: "/template/{id}"},
	"template.delete": {Method: "DELETE", Path: "/template/{id}"},

	// logs
	"logs.list":        {Method: "GET", Path: "/log-viewer/log"},
	"logs.level-count": {Method: "GET", Path: "/log-viewer/level-count"},
	"logs.templates":   {Method: "GET", Path: "/log-viewer/message-template"},
	"logs.search":      {Method: "GET", Path: "/log-viewer/log"},

	// server
	"server.status":        {Method: "GET", Path: "/server/status"},
	"server.info":          {Method: "GET", Path: "/server/information"},
	"server.config":        {Method: "GET", Path: "/server/configuration"},
	"server.troubleshoot":  {Method: "GET", Path: "/server/troubleshooting"},
	"server.upgrade-check": {Method: "GET", Path: "/server/upgrade-check"},

	// models-builder
	"models-builder.dashboard": {Method: "GET", Path: "/models-builder/dashboard"},
	"models-builder.status":    {Method: "GET", Path: "/models-builder/status"},
	"models-builder.build":     {Method: "POST", Path: "/models-builder/build", Response: &ObjectSchema{Type: "object", Description: "Server returns once generation has been queued (not waited on)"}},

	// member
	"member.list":   {Method: "GET", Path: "/filter/member", ExtraQuery: withFields},
	"member.search": {Method: "GET", Path: "/filter/member", ExtraQuery: withFields},
	"member.get":    {Method: "GET", Path: "/member/{id}", ExtraQuery: withFields},
	"member.create": {Method: "POST", Path: "/member"},
	"member.update": {Method: "PUT", Path: "/member/{id}"},
	"member.delete": {Method: "DELETE", Path: "/member/{id}"},

	// member-group
	"member-group.list": {Method: "GET", Path: "/member-group"},
	"member-group.get":  {Method: "GET", Path: "/member-group/{id}", ExtraQuery: withFields},

	// health — the CLI calls version-tolerant paths; the modern operations
	// are POST /health-check-group/{name}/check and /health-check/execute-action.
	"health.groups": {Method: "GET", Path: "/health-check-group"},
	"health.group":  {Method: "GET", Path: "/health-check-group/{name}"},
	"health.run":    {Manual: &rawSchema{Method: "GET", Path: "/health-check-group/{name}/run", PathParams: map[string]ParamSchema{"name": {Type: "string", Required: true}}, Response: &ObjectSchema{Type: "object", Description: "Version-dependent: newer servers expose POST /health-check-group/{name}/check instead"}}},
	"health.action": {Manual: &rawSchema{Method: "POST", Path: "/health-check/{actionId}", PathParams: map[string]ParamSchema{"actionId": {Type: "string", Required: true}}, Response: &ObjectSchema{Type: "object", Description: "Version-dependent: newer servers expose POST /health-check/execute-action instead"}}},

	// webhook
	"webhook.list":   {Method: "GET", Path: "/webhook"},
	"webhook.get":    {Method: "GET", Path: "/webhook/{id}"},
	"webhook.create": {Method: "POST", Path: "/webhook"},
	"webhook.update": {Method: "PUT", Path: "/webhook/{id}"},
	"webhook.delete": {Method: "DELETE", Path: "/webhook/{id}"},
	"webhook.events": {Method: "GET", Path: "/webhook/events"},
	"webhook.logs":   {Method: "GET", Path: "/webhook/logs"},

	// language
	"language.list":     {Method: "GET", Path: "/language"},
	"language.get":      {Method: "GET", Path: "/language/{isoCode}"},
	"language.create":   {Method: "POST", Path: "/language"},
	"language.update":   {Method: "PUT", Path: "/language/{isoCode}"},
	"language.delete":   {Method: "DELETE", Path: "/language/{isoCode}"},
	"language.default":  {Method: "GET", Path: "/item/language/default"},
	"language.cultures": {Method: "GET", Path: "/culture"},

	// user
	"user.list":        {Method: "GET", Path: "/filter/user"},
	"user.get":         {Method: "GET", Path: "/user/{id}", ExtraQuery: withFields},
	"user.create":      {Method: "POST", Path: "/user"},
	"user.invite":      {Method: "POST", Path: "/user/invite"},
	"user.update":      {Method: "PUT", Path: "/user/{id}"},
	"user.delete":      {Method: "DELETE", Path: "/user/{id}"},
	"user.enable":      {Method: "POST", Path: "/user/enable"},
	"user.disable":     {Method: "POST", Path: "/user/disable"},
	"user.unlock":      {Method: "POST", Path: "/user/unlock"},
	"user.set-groups":  {Method: "POST", Path: "/user/set-user-groups"},
	"user.current":     {Method: "GET", Path: "/user/current", ExtraQuery: withFields},
	"user.permissions": {Method: "GET", Path: "/user/current/permissions"},

	// user-group
	"user-group.list":         {Method: "GET", Path: "/user-group"},
	"user-group.get":          {Method: "GET", Path: "/user-group/{id}", ExtraQuery: withFields},
	"user-group.create":       {Method: "POST", Path: "/user-group"},
	"user-group.update":       {Method: "PUT", Path: "/user-group/{id}"},
	"user-group.delete":       {Method: "DELETE", Path: "/user-group/{id}"},
	"user-group.add-users":    {Method: "POST", Path: "/user-group/{id}/users"},
	"user-group.remove-users": {Method: "DELETE", Path: "/user-group/{id}/users"},
}

var Schemas = buildSchemas()
var Endpoints = endpointList()

func buildSchemas() map[string]Schema {
	result := make(map[string]Schema, len(endpointBindings))
	for endpoint, binding := range endpointBindings {
		raw, ok := resolveBinding(binding)
		if !ok {
			// A binding pointing at an operation missing from the vendored
			// document is a programming error caught by the package tests;
			// fall back to the bare method/path so the CLI stays usable.
			raw = rawSchema{Method: binding.Method, Path: binding.Path}
		}

		if len(binding.ExtraQuery) > 0 {
			merged := make(map[string]ParamSchema, len(raw.QueryParams)+len(binding.ExtraQuery))
			for key, value := range raw.QueryParams {
				merged[key] = value
			}
			for key, value := range binding.ExtraQuery {
				merged[key] = value
			}
			raw.QueryParams = merged
		}
		if binding.Response != nil {
			raw.Response = binding.Response
		}

		result[endpoint] = Schema{
			Endpoint:    endpoint,
			Method:      raw.Method,
			Path:        raw.Path,
			PathParams:  raw.PathParams,
			QueryParams: raw.QueryParams,
			RequestBody: raw.RequestBody,
			Response:    raw.Response,
		}
	}
	return result
}

func resolveBinding(binding endpointBinding) (rawSchema, bool) {
	if binding.Manual != nil {
		return *binding.Manual, true
	}
	raw, ok := openAPIOperations[binding.Method+" "+binding.Path]
	return raw, ok
}

func endpointList() []string {
	endpoints := make([]string, 0, len(Schemas))
	for endpoint := range Schemas {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	return endpoints
}
