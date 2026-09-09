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
	APIRoot     string                 `json:"apiRoot,omitempty"`
	Path        string                 `json:"path"`
	PathParams  map[string]ParamSchema `json:"pathParams,omitempty"`
	QueryParams map[string]ParamSchema `json:"queryParams,omitempty"`
	RequestBody *ObjectSchema          `json:"requestBody,omitempty"`
	Response    *ObjectSchema          `json:"response,omitempty"`
}

type rawSchema struct {
	Method string
	// APIRoot is the API mount when it differs from the default core
	// Management API (e.g. the Automate Management API).
	APIRoot     string
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
	fieldsQuery       = ParamSchema{Type: "string", Description: "Comma-separated field list (applied client-side by the CLI); supports dotted paths on document read commands"}
	documentTrimQuery = map[string]ParamSchema{
		"fields":   fieldsQuery,
		"summary":  {Type: "boolean", Description: "Return a compact CLI-side document summary shape"},
		"no-empty": {Type: "boolean", Description: "Omit null, empty string, empty array, and empty object values from trimmed output"},
		"full":     {Type: "boolean", Description: "Explicitly return the full payload; cannot be combined with --fields, --summary, or --no-empty"},
	}
	documentGetQuery = map[string]ParamSchema{
		"fields":    fieldsQuery,
		"summary":   {Type: "boolean", Description: "Return a compact CLI-side document summary shape"},
		"no-empty":  {Type: "boolean", Description: "Omit null, empty string, empty array, and empty object values from trimmed output"},
		"full":      {Type: "boolean", Description: "Explicitly return the full payload; cannot be combined with --fields, --summary, or --no-empty"},
		"with-urls": {Type: "boolean", Description: "Fetch GET /document/urls for the document and include the returned urlInfos as urls"},
	}
	withFields = map[string]ParamSchema{"fields": fieldsQuery}
)

var documentGrepSchema = &rawSchema{
	Method: "GET",
	Path:   "/tree/document/root + /tree/document/children + /document/{id}",
	QueryParams: map[string]ParamSchema{
		"substring":   {Type: "string", Required: true, Description: "Required positional substring or regex pattern"},
		"regex":       {Type: "boolean", Description: "Treat substring as a regular expression"},
		"ignore-case": {Type: "boolean", Description: "Case-insensitive matching"},
		"published":   {Type: "boolean", Description: "Scan only published snapshots"},
		"draft":       {Type: "boolean", Description: "Scan only current draft payloads"},
		"property":    {Type: "array", Description: "Restrict to property aliases; repeatable"},
		"doctype":     {Type: "array", Description: "Restrict to document type aliases; repeatable"},
		"start-id":    {Type: "string", Format: "uuid", Description: "Scan this document subtree instead of the full tree"},
		"concurrency": {Type: "number", Format: "int32", Description: "Maximum concurrent document fetches"},
	},
	Response: &ObjectSchema{
		Type:        "object",
		Description: "CLI workflow: walks the document tree, fetches draft and/or published document payloads, scans serialized property values client-side, and returns hits plus skipped fetches",
	},
}

var schemaDiffSchema = &rawSchema{
	Method: "GET",
	Path:   "profiles:{envA},{envB} + /tree/document-type/root + /document-type/{id} + /filter/data-type + /data-type/{id}",
	PathParams: map[string]ParamSchema{
		"envA": {Type: "string", Required: true, Description: "Configured profile/environment name for the left side of the comparison"},
		"envB": {Type: "string", Required: true, Description: "Configured profile/environment name for the right side of the comparison"},
	},
	QueryParams: map[string]ParamSchema{
		"entity":    {Type: "string", Description: "Comma-separated entity kinds to compare: doctype, datatype; defaults to both"},
		"include":   {Type: "array", Description: "Only include matching aliases/names; repeatable or comma-separated"},
		"exclude":   {Type: "array", Description: "Exclude matching aliases/names; repeatable or comma-separated"},
		"exit-zero": {Type: "boolean", Description: "Exit 0 even when schema differences are found"},
	},
	Response: &ObjectSchema{
		Type:        "object",
		Description: "CLI workflow: loads two configured profiles, fetches document types and data types from each, normalizes volatile IDs/order, and returns added/removed/changed schema differences",
	},
}

var automateCatalogueOperatorsSchema = &rawSchema{
	Method:  "GET",
	APIRoot: "/umbraco/automate/management/api/v1",
	Path:    "local:ConditionOperatorModel",
	QueryParams: map[string]ParamSchema{
		"fields": fieldsQuery,
	},
	Response: &ObjectSchema{
		Type:        "array",
		Description: "CLI-local catalogue from the Automate OpenAPI ConditionOperatorModel enum. Use the operator string in export/import/update JSON; deployUdaOperator documents the integer value used by Deploy .uda files.",
	},
}

// sortChildrenSchema documents the paired sort-children routes: the {id}
// route sorts under a parent, the root route (no path parameter) sorts
// top-level items — a single OpenAPI binding would report id as required.
func sortChildrenSchema(resource string, withCulture bool) *rawSchema {
	body := &ObjectSchema{
		Type:     "object",
		Required: []string{"field", "direction"},
		Properties: map[string]any{
			"field":     "enum: Name|CreateDate|UpdateDate",
			"direction": "enum: Ascending|Descending",
		},
	}
	if withCulture {
		body.Properties["culture"] = "string (sort by this culture's variant name)"
	}
	return &rawSchema{
		Method: "PUT",
		Path:   "/" + resource + "/root/sort-children | /" + resource + "/{id}/sort-children",
		PathParams: map[string]ParamSchema{
			"id": {Type: "string", Format: "uuid", Description: "Parent id; omit the positional argument to sort root-level items via the root route"},
		},
		RequestBody: body,
		Response:    &ObjectSchema{Type: "object", Description: "Umbraco 18.1+; sorts every child of the parent server-side by field"},
	}
}

var deployStatusSchema = &rawSchema{
	Method: "GET",
	Path:   "local: <uda-dir>/*.uda + per-kind Management API lookups (/data-type/{id}, /document-type/{id}, /template/{id}, ...)",
	QueryParams: map[string]ParamSchema{
		"uda-dir":         {Type: "string", Description: "Directory holding the .uda artifacts (default umbraco/Deploy/Revision)"},
		"kind":            {Type: "array", Description: "Only compare these artifact kinds (Udi entity types); repeatable"},
		"flag-step-alias": {Type: "array", Description: "Flag automations whose steps carry this action-alias substring; repeatable"},
		"exit-zero":       {Type: "boolean", Description: "Exit 0 even when drift or missing entities are found"},
		"concurrency":     {Type: "number", Format: "int32", Description: "Maximum concurrent environment lookups (default 8)"},
	},
	Response: &ObjectSchema{
		Type:        "object",
		Description: "CLI workflow, read-only: parses Deploy .uda artifacts (BOM-tolerant, Udi-discriminated) and compares each against the environment, reporting in-sync | drifted (with fields) | missing-remote | unknown | error per artifact plus a summary; exit 7 on drift/missing. Automate artifacts degrade to unknown where that API is unavailable, never false in-sync.",
	},
}

var deployWatchSchema = &rawSchema{
	Method: "GET",
	Path:   "poll: /security/back-office/token (unauthenticated probe) + /log-viewer/log + /indexer + <public-url>/<health-path>",
	QueryParams: map[string]ParamSchema{
		"health-path":       {Type: "array", Description: "Public paths that must return 2xx for serving/verified; repeatable, default /"},
		"public-url":        {Type: "string", Description: "Public host for health paths when it differs from the management base URL"},
		"interval":          {Type: "string", Description: "Poll interval (Go duration, default 5s)"},
		"timeout":           {Type: "string", Description: "Give up after this long without verification (default 30m; exit 6, status unknown)"},
		"escalation":        {Type: "string", Description: "Sustained downtime or post-landing health failure beyond this fails the watch (default 10m; exit 5)"},
		"settle":            {Type: "string", Description: "How long the environment must stay healthy after everything first looks good before verified (default 90s; 0 = single-sample)"},
		"heartbeat":         {Type: "string", Description: "Still-alive line interval on stderr; 0 disables (default 1m)"},
		"json":              {Type: "boolean", Description: "Emit phase transitions as NDJSON"},
		"skip-index-verify": {Type: "boolean", Description: "Do not require Examine indexes healthy for verified"},
	},
	Response: &ObjectSchema{
		Type:        "object",
		Description: "CLI workflow: baselines ProcessId/MachineName, health paths, and index health, then polls for state deltas only a deployment can cause, emitting phase transitions baseline → restarting → app-alive → serving → landed → settling → verified | failed | timeout (settle-interrupted marks disturbances that restart the settle window). Read-only; no pipeline or portal API involved.",
	},
}

var endpointBindings = map[string]endpointBinding{
	// schema
	"schema.diff": {Manual: schemaDiffSchema},

	// element (Umbraco 18.1+ element library content; whole group requires 18.1+)
	"element.list":                    {Method: "GET", Path: "/tree/element/root"},
	"element.children":                {Method: "GET", Path: "/tree/element/children"},
	"element.ancestors":               {Method: "GET", Path: "/tree/element/ancestors"},
	"element.search":                  {Method: "GET", Path: "/item/element/search"},
	"element.get":                     {Method: "GET", Path: "/element/{id}", ExtraQuery: withFields},
	"element.published":               {Method: "GET", Path: "/element/{id}/published", ExtraQuery: withFields},
	"element.create":                  {Method: "POST", Path: "/element", Response: &ObjectSchema{Type: "object", Description: "--publish posts to POST /element/create-and-publish instead, with --culture filling culturesToPublish (empty list = invariant)"}},
	"element.update":                  {Method: "PUT", Path: "/element/{id}", Response: &ObjectSchema{Type: "object", Description: "--save-and-publish targets PUT /element/{id}/update-and-publish atomically"}},
	"element.publish":                 {Method: "PUT", Path: "/element/{id}/publish"},
	"element.unpublish":               {Method: "PUT", Path: "/element/{id}/unpublish"},
	"element.copy":                    {Method: "POST", Path: "/element/{id}/copy"},
	"element.move":                    {Method: "PUT", Path: "/element/{id}/move"},
	"element.trash":                   {Method: "PUT", Path: "/element/{id}/move-to-recycle-bin"},
	"element.delete":                  {Method: "DELETE", Path: "/element/{id}"},
	"element.audit-log":               {Method: "GET", Path: "/element/{id}/audit-log"},
	"element.references":              {Method: "GET", Path: "/element/{id}/referenced-by"},
	"element.referenced-descendants":  {Method: "GET", Path: "/element/folder/{id}/referenced-descendants"},
	"element.are-referenced":          {Method: "GET", Path: "/element/are-referenced"},
	"element.restore":                 {Method: "PUT", Path: "/recycle-bin/element/{id}/restore"},
	"element.bin.list":                {Method: "GET", Path: "/recycle-bin/element/root"},
	"element.bin.children":            {Method: "GET", Path: "/recycle-bin/element/children"},
	"element.bin.original-parent":     {Method: "GET", Path: "/recycle-bin/element/{id}/original-parent"},
	"element.bin.delete":              {Method: "DELETE", Path: "/recycle-bin/element/{id}"},
	"element.bin.empty":               {Method: "DELETE", Path: "/recycle-bin/element"},
	"element.version.list":            {Method: "GET", Path: "/element-version"},
	"element.version.get":             {Method: "GET", Path: "/element-version/{id}"},
	"element.version.rollback":        {Method: "POST", Path: "/element-version/{id}/rollback"},
	"element.version.prevent-cleanup": {Method: "PUT", Path: "/element-version/{id}/prevent-cleanup"},

	// deploy (effect-based observation composites)
	"deploy.watch":  {Manual: deployWatchSchema},
	"deploy.status": {Manual: deployStatusSchema},

	// document
	"document.get":                        {Method: "GET", Path: "/document/{id}", ExtraQuery: documentGetQuery},
	"document.urls":                       {Method: "GET", Path: "/document/urls", ExtraQuery: map[string]ParamSchema{"culture": {Type: "string", Description: "CLI-side filter for a single culture"}, "absolute": {Type: "boolean", Description: "Resolve returned URLs against the configured site host"}}},
	"document.root":                       {Method: "GET", Path: "/tree/document/root", ExtraQuery: documentTrimQuery},
	"document.children":                   {Method: "GET", Path: "/tree/document/children", ExtraQuery: documentTrimQuery},
	"document.ancestors":                  {Method: "GET", Path: "/tree/document/ancestors"},
	"document.search":                     {Method: "GET", Path: "/item/document/search", ExtraQuery: documentTrimQuery},
	"document.grep":                       {Manual: documentGrepSchema},
	"document.create":                     {Method: "POST", Path: "/document", Response: &ObjectSchema{Type: "object", Description: "--publish posts to POST /document/create-and-publish instead (Umbraco 18.1+), with --culture filling culturesToPublish (empty list = invariant)"}},
	"document.update":                     {Method: "PUT", Path: "/document/{id}", Response: &ObjectSchema{Type: "object", Description: "--save-and-publish targets PUT /document/{id}/update-and-publish atomically (Umbraco 18.1+), falling back to separate update+publish calls on older servers"}},
	"document.update-properties":          {Method: "PUT", Path: "/document/{id}", Response: &ObjectSchema{Type: "object", Description: "CLI convenience wrapper that fetches, merges, and writes the full document payload"}},
	"document.publish":                    {Method: "PUT", Path: "/document/{id}/publish"},
	"document.unpublish":                  {Method: "PUT", Path: "/document/{id}/unpublish"},
	"document.publish-descendants":        {Method: "PUT", Path: "/document/{id}/publish-with-descendants", Response: &ObjectSchema{Type: "object", Description: "Asynchronous; carries a taskId for publish-descendants-result"}},
	"document.publish-descendants-result": {Method: "GET", Path: "/document/{id}/publish-with-descendants/result/{taskId}"},
	"document.sort":                       {Method: "PUT", Path: "/document/sort"},
	"document.sort-children":              {Manual: sortChildrenSchema("document", true)},
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
	"media.replace-file":           {Manual: &rawSchema{Method: "PUT", Path: "/media/{id}", RequestBody: &ObjectSchema{Type: "object", Description: "CLI workflow: fetch current item, multipart temporary-file upload, PUT with the file property rewritten, then re-fetch to verify the item was not emptied"}}},
	"media.restore-backup":         {Manual: &rawSchema{Method: "PUT", Path: "/media/{id}", RequestBody: &ObjectSchema{Type: "object", Description: "CLI workflow: PUT the entity captured by --backup back onto the item, then verify"}}},
	"media.update":                 {Method: "PUT", Path: "/media/{id}"},
	"media.move":                   {Method: "PUT", Path: "/media/{id}/move"},
	"media.sort":                   {Method: "PUT", Path: "/media/sort"},
	"media.sort-children":          {Manual: sortChildrenSchema("media", false)},
	"media.delete":                 {Method: "DELETE", Path: "/media/{id}"},
	"media.trash":                  {Method: "PUT", Path: "/media/{id}/move-to-recycle-bin"},
	"media.references":             {Method: "GET", Path: "/media/{id}/referenced-by"},
	"media.referenced-descendants": {Method: "GET", Path: "/media/{id}/referenced-descendants"},
	"media.are-referenced":         {Method: "GET", Path: "/media/are-referenced"},

	// doctype
	"doctype.get":                {Method: "GET", Path: "/document-type/{id}", ExtraQuery: withFields},
	"doctype.list":               {Method: "GET", Path: "/tree/document-type/root", ExtraQuery: withFields},
	"doctype.root":               {Method: "GET", Path: "/tree/document-type/root"},
	"doctype.children":           {Method: "GET", Path: "/tree/document-type/children"},
	"doctype.allowed-in-library": {Method: "GET", Path: "/document-type/allowed-in-library"},
	"doctype.search":             {Method: "GET", Path: "/item/document-type/search"},
	"doctype.create":             {Method: "POST", Path: "/document-type"},
	"doctype.update":             {Method: "PUT", Path: "/document-type/{id}"},
	"doctype.copy":               {Method: "POST", Path: "/document-type/{id}/copy"},
	"doctype.move":               {Method: "PUT", Path: "/document-type/{id}/move"},
	"doctype.delete":             {Method: "DELETE", Path: "/document-type/{id}"},

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
	"logs.errors":      {Method: "GET", Path: "/log-viewer/log", ExtraQuery: map[string]ParamSchema{"since": {Type: "string", Description: "Start of the window (ISO/RFC3339); default 24h ago"}, "until": {Type: "string", Description: "End of the window; default now"}, "distinct": {Type: "boolean", Description: "Group Error/Fatal entries into fingerprinted error classes"}, "suppress": {Type: "array", Description: "Known-chronic class fingerprints to drop; repeatable"}, "suppress-contains": {Type: "array", Description: "Drop classes whose template/exception contains this substring; repeatable"}, "max-entries": {Type: "number", Format: "int32", Description: "Maximum entries scanned in the window (default 10000)"}}, Response: &ObjectSchema{Type: "object", Description: "Error+Fatal entries; --distinct returns {classes, totalEntries, suppressedGroups, since} with classes sorted newest-first-seen"}},

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

	// health — run and action call the modern operations first and fall back
	// to the legacy routes (GET .../run, POST /health-check/{actionId}) on 404.
	"health.groups": {Method: "GET", Path: "/health-check-group"},
	"health.group":  {Method: "GET", Path: "/health-check-group/{name}"},
	"health.run":    {Method: "POST", Path: "/health-check-group/{name}/check", Response: &ObjectSchema{Type: "object", Description: "Falls back to GET /health-check-group/{name}/run on older servers"}},
	"health.action": {Method: "POST", Path: "/health-check/execute-action", Response: &ObjectSchema{Type: "object", Description: "The positional health-check id fills healthCheck.id when --json omits it; falls back to POST /health-check/{actionId} on older servers"}},

	// published-cache — the CLI's status command falls back to the legacy
	// /published-cache/status route on older servers.
	"published-cache.status":  {Method: "GET", Path: "/published-cache/rebuild/status"},
	"published-cache.rebuild": {Method: "POST", Path: "/published-cache/rebuild"},
	"published-cache.reload":  {Method: "POST", Path: "/published-cache/reload"},

	// media-type / member-type (schema type groups sharing the doctype folder semantics)
	"mediatype.list":      {Method: "GET", Path: "/tree/media-type/root"},
	"mediatype.get":       {Method: "GET", Path: "/media-type/{id}"},
	"mediatype.children":  {Method: "GET", Path: "/tree/media-type/children"},
	"mediatype.search":    {Method: "GET", Path: "/item/media-type/search"},
	"mediatype.create":    {Method: "POST", Path: "/media-type"},
	"mediatype.update":    {Method: "PUT", Path: "/media-type/{id}"},
	"mediatype.delete":    {Method: "DELETE", Path: "/media-type/{id}"},
	"mediatype.export":    {Method: "GET", Path: "/media-type/{id}/export"},
	"membertype.list":     {Method: "GET", Path: "/tree/member-type/root"},
	"membertype.get":      {Method: "GET", Path: "/member-type/{id}"},
	"membertype.children": {Method: "GET", Path: "/tree/member-type/children"},
	"membertype.search":   {Method: "GET", Path: "/item/member-type/search"},
	"membertype.create":   {Method: "POST", Path: "/member-type"},
	"membertype.update":   {Method: "PUT", Path: "/member-type/{id}"},
	"membertype.delete":   {Method: "DELETE", Path: "/member-type/{id}"},
	"membertype.export":   {Method: "GET", Path: "/member-type/{id}/export"},

	// recycle bin (document/media symmetric)
	"document.bin.list":            {Method: "GET", Path: "/recycle-bin/document/root"},
	"document.bin.children":        {Method: "GET", Path: "/recycle-bin/document/children"},
	"document.bin.original-parent": {Method: "GET", Path: "/recycle-bin/document/{id}/original-parent"},
	"document.bin.delete":          {Method: "DELETE", Path: "/recycle-bin/document/{id}"},
	"document.bin.empty":           {Method: "DELETE", Path: "/recycle-bin/document"},
	"media.restore":                {Method: "PUT", Path: "/recycle-bin/media/{id}/restore"},
	"media.bin.list":               {Method: "GET", Path: "/recycle-bin/media/root"},
	"media.bin.children":           {Method: "GET", Path: "/recycle-bin/media/children"},
	"media.bin.original-parent":    {Method: "GET", Path: "/recycle-bin/media/{id}/original-parent"},
	"media.bin.delete":             {Method: "DELETE", Path: "/recycle-bin/media/{id}"},
	"media.bin.empty":              {Method: "DELETE", Path: "/recycle-bin/media"},

	// indexer
	"indexer.list":    {Method: "GET", Path: "/indexer"},
	"indexer.get":     {Method: "GET", Path: "/indexer/{indexName}"},
	"indexer.rebuild": {Method: "POST", Path: "/indexer/{indexName}/rebuild"},

	// redirect
	"redirect.list":    {Method: "GET", Path: "/redirect-management"},
	"redirect.get":     {Method: "GET", Path: "/redirect-management/{id}"},
	"redirect.delete":  {Method: "DELETE", Path: "/redirect-management/{id}"},
	"redirect.status":  {Method: "GET", Path: "/redirect-management/status"},
	"redirect.enable":  {Method: "POST", Path: "/redirect-management/status"},
	"redirect.disable": {Method: "POST", Path: "/redirect-management/status"},

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
	"user.list":         {Method: "GET", Path: "/filter/user"},
	"user.get":          {Method: "GET", Path: "/user/{id}", ExtraQuery: withFields, Response: &ObjectSchema{Type: "object", Description: "Several IDs fetch via GET /user/batch in one round trip (Umbraco 18.1+)"}},
	"user.set-language": {Method: "PUT", Path: "/user/current/profile", Response: &ObjectSchema{Type: "object", Description: "Sets the backoffice UI language of the authenticated account (Umbraco 18.1+)"}},
	"user.create":       {Method: "POST", Path: "/user"},
	"user.invite":       {Method: "POST", Path: "/user/invite"},
	"user.update":       {Method: "PUT", Path: "/user/{id}"},
	"user.delete":       {Method: "DELETE", Path: "/user/{id}"},
	"user.enable":       {Method: "POST", Path: "/user/enable"},
	"user.disable":      {Method: "POST", Path: "/user/disable"},
	"user.unlock":       {Method: "POST", Path: "/user/unlock"},
	"user.set-groups":   {Method: "POST", Path: "/user/set-user-groups"},
	"user.current":      {Method: "GET", Path: "/user/current", ExtraQuery: withFields},
	"user.permissions":  {Method: "GET", Path: "/user/current/permissions"},

	// automate (gated behind UMBRACO_CLI_ENABLE_AUTOMATE; served from the
	// Automate Management API mount, which generated entries carry as APIRoot)
	"automate.catalogue.actions":                {Method: "GET", Path: "/catalogue/actions"},
	"automate.catalogue.triggers":               {Method: "GET", Path: "/catalogue/triggers"},
	"automate.catalogue.connection-types":       {Method: "GET", Path: "/catalogue/connection-types"},
	"automate.catalogue.control-flows":          {Method: "GET", Path: "/catalogue/control-flows"},
	"automate.catalogue.notification-channels":  {Method: "GET", Path: "/catalogue/notification-channels"},
	"automate.catalogue.webhook-authenticators": {Method: "GET", Path: "/catalogue/webhook-authenticators"},
	"automate.catalogue.operators":              {Manual: automateCatalogueOperatorsSchema},
	"automate.catalogue.step-types":             {Method: "GET", Path: "/catalogue/step-types"},
	"automate.catalogue.output-schema":          {Method: "POST", Path: "/catalogue/step-types/{alias}/output-schema"},
	"automate.automation.list":                  {Method: "GET", Path: "/automations"},
	"automate.automation.get":                   {Method: "GET", Path: "/automations/{id}"},
	"automate.automation.runs":                  {Method: "GET", Path: "/automations/{id}/runs"},
	"automate.automation.trigger":               {Method: "POST", Path: "/automations/{id}/trigger"},
	"automate.automation.export":                {Method: "GET", Path: "/automations/{id}/export"},
	"automate.run.get":                          {Method: "GET", Path: "/runs/{id}"},
	"automate.run.replay":                       {Method: "POST", Path: "/runs/{id}/replay"},
	"automate.run.resume":                       {Method: "POST", Path: "/runs/{id}/resume"},
	"automate.run.suspend":                      {Method: "POST", Path: "/runs/{id}/suspend"},
	"automate.run.terminate":                    {Method: "POST", Path: "/runs/{id}/terminate"},
	"automate.approvals.pending":                {Method: "GET", Path: "/approvals/pending"},
	"automate.approvals.decide":                 {Method: "POST", Path: "/approvals/{runId}/steps/{stepId}/decision"},
	"automate.metrics.summary":                  {Method: "GET", Path: "/metrics"},
	"automate.metrics.by-automation":            {Method: "GET", Path: "/metrics/by-automation"},
	"automate.workspace.list":                   {Method: "GET", Path: "/workspaces"},
	"automate.workspace.get":                    {Method: "GET", Path: "/workspaces/{id}"},
	"automate.workspace.create":                 {Method: "POST", Path: "/workspaces"},
	"automate.workspace.update":                 {Method: "PUT", Path: "/workspaces/{id}"},
	"automate.workspace.delete":                 {Method: "DELETE", Path: "/workspaces/{id}"},
	"automate.workspace.group.list":             {Method: "GET", Path: "/workspaces/{id}/groups"},
	"automate.workspace.group.get":              {Method: "GET", Path: "/workspaces/{id}/groups/{groupId}"},
	"automate.workspace.group.add":              {Method: "POST", Path: "/workspaces/{id}/groups"},
	"automate.workspace.group.update":           {Method: "PUT", Path: "/workspaces/{id}/groups/{groupId}"},
	"automate.workspace.group.remove":           {Method: "DELETE", Path: "/workspaces/{id}/groups/{groupId}"},
	"automate.connection.list":                  {Method: "GET", Path: "/connections"},
	"automate.connection.get":                   {Method: "GET", Path: "/connections/{id}"},
	"automate.connection.create":                {Method: "POST", Path: "/connections"},
	"automate.connection.update":                {Method: "PUT", Path: "/connections/{id}"},
	"automate.connection.delete":                {Method: "DELETE", Path: "/connections/{id}"},
	"automate.connection.test":                  {Method: "POST", Path: "/connections/{id}/test"},
	"automate.automation.create":                {Method: "POST", Path: "/automations"},
	"automate.automation.update":                {Method: "PUT", Path: "/automations/{id}"},
	"automate.automation.delete":                {Method: "DELETE", Path: "/automations/{id}"},
	"automate.automation.publish":               {Method: "POST", Path: "/automations/{id}/publish"},
	"automate.automation.unpublish":             {Method: "POST", Path: "/automations/{id}/unpublish"},
	"automate.automation.re-enable":             {Method: "POST", Path: "/automations/{id}/re-enable"},
	"automate.automation.ancestors":             {Method: "GET", Path: "/automations/{id}/ancestors"},
	"automate.automation.validate":              {Method: "POST", Path: "/automations/import/validate"},
	"automate.automation.import":                {Method: "POST", Path: "/automations/import"},
	"automate.automation.import-update":         {Method: "PUT", Path: "/automations/{id}/import"},
	"automate.version-history.types":            {Method: "GET", Path: "/version-history/supported-types"},
	"automate.version-history.list":             {Method: "GET", Path: "/version-history/{entityType}/{entityId}"},
	"automate.version-history.get":              {Method: "GET", Path: "/version-history/{entityType}/{entityId}/{entityVersion}"},
	"automate.version-history.compare":          {Method: "GET", Path: "/version-history/{entityType}/{entityId}/{fromEntityVersion}/compare/{toEntityVersion}"},
	"automate.version-history.rollback":         {Method: "POST", Path: "/version-history/{entityType}/{entityId}/{entityVersion}/rollback"},

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
			APIRoot:     raw.APIRoot,
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
