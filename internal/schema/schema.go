package schema

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

var (
	idParam            = ParamSchema{Type: "string", Format: "uuid", Required: true}
	fieldsQuery        = ParamSchema{Type: "string", Description: "Comma-separated field list"}
	genericRequestBody = &ObjectSchema{Type: "object", Description: "Raw JSON payload accepted by the endpoint"}
)

var rawSchemas = map[string]rawSchema{
	// document (15)
	"document.get":               {Method: "GET", Path: "/document/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"document.root":              {Method: "GET", Path: "/tree/document/root", QueryParams: map[string]ParamSchema{"fields": fieldsQuery, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"document.children":          {Method: "GET", Path: "/tree/document/children", QueryParams: map[string]ParamSchema{"parentId": {Type: "string", Format: "uuid", Required: true}, "fields": fieldsQuery, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"document.ancestors":         {Method: "GET", Path: "/tree/document/ancestors", QueryParams: map[string]ParamSchema{"descendantId": {Type: "string", Format: "uuid", Required: true}}},
	"document.search":            {Method: "GET", Path: "/item/document/search", QueryParams: map[string]ParamSchema{"query": {Type: "string"}, "skip": {Type: "number"}, "take": {Type: "number"}, "parentId": {Type: "string", Format: "uuid"}, "culture": {Type: "string"}, "dataTypeId": {Type: "string", Format: "uuid"}, "trashed": {Type: "boolean"}, "allowedDocumentTypes": {Type: "array", Format: "uuid"}}},
	"document.create":            {Method: "POST", Path: "/document", RequestBody: genericRequestBody},
	"document.update":            {Method: "PUT", Path: "/document/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"document.update-properties": {Method: "PUT", Path: "/document/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody, Response: &ObjectSchema{Type: "object", Description: "CLI convenience wrapper that fetches, merges, and writes the full document payload"}},
	"document.publish":           {Method: "PUT", Path: "/document/{id}/publish", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"document.unpublish":         {Method: "PUT", Path: "/document/{id}/unpublish", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"document.copy":              {Method: "POST", Path: "/document/{id}/copy", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"document.move":              {Method: "POST", Path: "/document/{id}/move", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"document.delete":            {Method: "DELETE", Path: "/document/{id}", PathParams: map[string]ParamSchema{"id": idParam}},
	"document.trash":             {Method: "POST", Path: "/document/{id}/move-to-recycle-bin", PathParams: map[string]ParamSchema{"id": idParam}},
	"document.restore":           {Method: "POST", Path: "/document/{id}/restore", PathParams: map[string]ParamSchema{"id": idParam}},
	"document.references":             {Method: "GET", Path: "/document/{id}/referenced-by", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"document.referenced-descendants": {Method: "GET", Path: "/document/{id}/referenced-descendants", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"document.are-referenced":         {Method: "GET", Path: "/document/are-referenced", QueryParams: map[string]ParamSchema{"id": {Type: "array", Format: "uuid", Required: true, Description: "Repeat the id query parameter for each document"}}},

	// dictionary (12)
	"dictionary.list":      {Method: "GET", Path: "/dictionary", QueryParams: map[string]ParamSchema{"filter": {Type: "string"}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"dictionary.get":       {Method: "GET", Path: "/dictionary/{id}", PathParams: map[string]ParamSchema{"id": idParam}},
	"dictionary.get-many":  {Method: "GET", Path: "/item/dictionary", QueryParams: map[string]ParamSchema{"id": {Type: "array", Format: "uuid", Description: "Repeat the id query parameter for each item"}}},
	"dictionary.create":    {Method: "POST", Path: "/dictionary", RequestBody: genericRequestBody},
	"dictionary.update":    {Method: "PUT", Path: "/dictionary/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"dictionary.delete":    {Method: "DELETE", Path: "/dictionary/{id}", PathParams: map[string]ParamSchema{"id": idParam}},
	"dictionary.import":    {Method: "POST", Path: "/dictionary/import", RequestBody: genericRequestBody},
	"dictionary.export":    {Method: "GET", Path: "/dictionary/{id}/export", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"includeChildren": {Type: "boolean"}}},
	"dictionary.move":      {Method: "PUT", Path: "/dictionary/{id}/move", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"dictionary.root":      {Method: "GET", Path: "/tree/dictionary/root", QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"dictionary.children":  {Method: "GET", Path: "/tree/dictionary/children", QueryParams: map[string]ParamSchema{"parentId": {Type: "string", Format: "uuid", Required: true}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"dictionary.ancestors": {Method: "GET", Path: "/tree/dictionary/ancestors", QueryParams: map[string]ParamSchema{"descendantId": {Type: "string", Format: "uuid", Required: true}}},

	// media (12)
	"media.get":           {Method: "GET", Path: "/media/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"media.root":          {Method: "GET", Path: "/tree/media/root", QueryParams: map[string]ParamSchema{"fields": fieldsQuery, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"media.children":      {Method: "GET", Path: "/tree/media/children", QueryParams: map[string]ParamSchema{"parentId": {Type: "string", Format: "uuid", Required: true}, "fields": fieldsQuery, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"media.search":        {Method: "GET", Path: "/item/media/search", QueryParams: map[string]ParamSchema{"query": {Type: "string"}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"media.urls":          {Method: "GET", Path: "/media/{id}/urls", PathParams: map[string]ParamSchema{"id": idParam}},
	"media.create":        {Method: "POST", Path: "/media", RequestBody: genericRequestBody},
	"media.create-folder": {Method: "POST", Path: "/media/folder", RequestBody: genericRequestBody},
	"media.upload":        {Method: "POST", Path: "/temporary-file", RequestBody: &ObjectSchema{Type: "object", Description: "CLI workflow: multipart temporary-file upload followed by media create"}},
	"media.update":        {Method: "PUT", Path: "/media/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"media.move":          {Method: "POST", Path: "/media/{id}/move", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"media.delete":        {Method: "DELETE", Path: "/media/{id}", PathParams: map[string]ParamSchema{"id": idParam}},
	"media.trash":         {Method: "POST", Path: "/media/{id}/move-to-recycle-bin", PathParams: map[string]ParamSchema{"id": idParam}},
	"media.references":             {Method: "GET", Path: "/media/{id}/referenced-by", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"media.referenced-descendants": {Method: "GET", Path: "/media/{id}/referenced-descendants", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"media.are-referenced":         {Method: "GET", Path: "/media/are-referenced", QueryParams: map[string]ParamSchema{"id": {Type: "array", Format: "uuid", Required: true, Description: "Repeat the id query parameter for each media item"}}},

	// doctype (10)
	"doctype.get":      {Method: "GET", Path: "/document-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"doctype.list":     {Method: "GET", Path: "/document-type", QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"doctype.root":     {Method: "GET", Path: "/tree/document-type/root", QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"doctype.children": {Method: "GET", Path: "/tree/document-type/children", QueryParams: map[string]ParamSchema{"parentId": {Type: "string", Format: "uuid", Required: true}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"doctype.search":   {Method: "GET", Path: "/item/document-type/search", QueryParams: map[string]ParamSchema{"query": {Type: "string"}}},
	"doctype.create":   {Method: "POST", Path: "/document-type", RequestBody: genericRequestBody},
	"doctype.update":   {Method: "PUT", Path: "/document-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"doctype.copy":     {Method: "POST", Path: "/document-type/{id}/copy", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"doctype.move":     {Method: "POST", Path: "/document-type/{id}/move", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"doctype.delete":   {Method: "DELETE", Path: "/document-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}},

	// datatype (8)
	"datatype.get":     {Method: "GET", Path: "/data-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"datatype.list":    {Method: "GET", Path: "/filter/data-type", QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"datatype.root":    {Method: "GET", Path: "/tree/data-type/root", QueryParams: map[string]ParamSchema{"skip": {Type: "number"}, "take": {Type: "number"}}},
	"datatype.search":  {Method: "GET", Path: "/item/data-type/search", QueryParams: map[string]ParamSchema{"query": {Type: "string"}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"datatype.is-used": {Method: "GET", Path: "/data-type/{id}/is-used", PathParams: map[string]ParamSchema{"id": idParam}},
	"datatype.create":  {Method: "POST", Path: "/data-type", RequestBody: genericRequestBody},
	"datatype.update":  {Method: "PUT", Path: "/data-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"datatype.delete":  {Method: "DELETE", Path: "/data-type/{id}", PathParams: map[string]ParamSchema{"id": idParam}},

	// template (6)
	"template.get":    {Method: "GET", Path: "/template/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"template.root":   {Method: "GET", Path: "/tree/template/root"},
	"template.search": {Method: "GET", Path: "/item/template/search", QueryParams: map[string]ParamSchema{"query": {Type: "string"}}},
	"template.create": {Method: "POST", Path: "/template", RequestBody: genericRequestBody},
	"template.update": {Method: "PUT", Path: "/template/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"template.delete": {Method: "DELETE", Path: "/template/{id}", PathParams: map[string]ParamSchema{"id": idParam}},

	// logs (5)
	"logs.list":        {Method: "GET", Path: "/log-viewer/log", QueryParams: map[string]ParamSchema{"startDate": {Type: "string", Format: "date-time"}, "endDate": {Type: "string", Format: "date-time"}, "skip": {Type: "number"}, "take": {Type: "number"}, "filterExpression": {Type: "string"}, "logLevel": {Type: "array", Description: "Repeat the logLevel query parameter for each level"}}},
	"logs.level-count": {Method: "GET", Path: "/log-viewer/level-count", QueryParams: map[string]ParamSchema{"startDate": {Type: "string", Format: "date-time"}, "endDate": {Type: "string", Format: "date-time"}}},
	"logs.templates":   {Method: "GET", Path: "/log-viewer/message-template", QueryParams: map[string]ParamSchema{"startDate": {Type: "string", Format: "date-time"}, "endDate": {Type: "string", Format: "date-time"}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"logs.search":      {Method: "GET", Path: "/log-viewer/log", QueryParams: map[string]ParamSchema{"startDate": {Type: "string", Format: "date-time"}, "endDate": {Type: "string", Format: "date-time"}, "skip": {Type: "number"}, "take": {Type: "number"}, "filterExpression": {Type: "string"}, "logLevel": {Type: "array", Description: "Repeat the logLevel query parameter for each level"}}},

	// server (5)
	"server.status":        {Method: "GET", Path: "/server/status"},
	"server.info":          {Method: "GET", Path: "/server/information"},
	"server.config":        {Method: "GET", Path: "/server/configuration"},
	"server.troubleshoot":  {Method: "GET", Path: "/server/troubleshooting"},
	"server.upgrade-check": {Method: "GET", Path: "/server/upgrade-check"},

	// models-builder (3)
	"models-builder.dashboard": {Method: "GET", Path: "/models-builder/dashboard"},
	"models-builder.status":    {Method: "GET", Path: "/models-builder/status"},
	"models-builder.build":     {Method: "POST", Path: "/models-builder/build", RequestBody: &ObjectSchema{Type: "object", Description: "Empty body; the build is triggered by the POST itself. Server returns once generation has been queued (not waited on)."}},

	// member (6 schema-backed endpoints; update-properties/approve/unlock/set-groups are convenience commands that piggy-back on member.update)
	"member.list":   {Method: "GET", Path: "/filter/member", QueryParams: map[string]ParamSchema{"filter": {Type: "string", Description: "Substring filter against username/email"}, "skip": {Type: "number"}, "take": {Type: "number"}}},
	"member.search": {Method: "GET", Path: "/filter/member", QueryParams: map[string]ParamSchema{"filter": {Type: "string", Required: true, Description: "Substring filter against username/email"}, "take": {Type: "number"}}},
	"member.get":    {Method: "GET", Path: "/member/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},
	"member.create": {Method: "POST", Path: "/member", RequestBody: genericRequestBody},
	"member.update": {Method: "PUT", Path: "/member/{id}", PathParams: map[string]ParamSchema{"id": idParam}, RequestBody: genericRequestBody},
	"member.delete": {Method: "DELETE", Path: "/member/{id}", PathParams: map[string]ParamSchema{"id": idParam}},

	// member-group (2)
	"member-group.list": {Method: "GET", Path: "/member-group"},
	"member-group.get":  {Method: "GET", Path: "/member-group/{id}", PathParams: map[string]ParamSchema{"id": idParam}, QueryParams: map[string]ParamSchema{"fields": fieldsQuery}},

	// health (4)
	"health.groups": {Method: "GET", Path: "/health-check-group"},
	"health.group":  {Method: "GET", Path: "/health-check-group/{name}", PathParams: map[string]ParamSchema{"name": {Type: "string", Required: true}}},
	"health.run":    {Method: "GET", Path: "/health-check-group/{name}/run", PathParams: map[string]ParamSchema{"name": {Type: "string", Required: true}}},
	"health.action": {Method: "POST", Path: "/health-check/{actionId}", PathParams: map[string]ParamSchema{"actionId": {Type: "string", Required: true}}},
}

var Schemas = buildSchemas()
var Endpoints = endpointList()
var Templates = map[string]any{
	"document.create": map[string]any{
		"id":           "<uuid, optional; generated by CLI when omitted>",
		"documentType": map[string]any{"id": "<document type id, required>"},
		"parent":       map[string]any{"id": "<parent document id, optional>"},
		"variants": []any{map[string]any{
			"name":    "<string, required>",
			"culture": "<culture, optional>",
		}},
		"values": []any{map[string]any{
			"alias":   "<property alias>",
			"value":   "<property value>",
			"culture": "<culture, optional>",
		}},
	},
	"doctype.create": map[string]any{
		"id":               "<uuid, optional; generated by CLI when omitted>",
		"name":             "<string, required>",
		"alias":            "<camelCase string, required>",
		"description":      "<string, optional>",
		"icon":             "<icon name, e.g. icon-document>",
		"allowedAsRoot":    "<boolean>",
		"variesByCulture":  "<boolean>",
		"variesBySegment":  "<boolean>",
		"isElement":        "<boolean, optional; true creates an element type (no own URL, intended for Block List/Grid)>",
		"allowedTemplates": "<array of {id} refs, optional; only valid for non-element types>",
		"defaultTemplate":  "<map {id} ref, optional; must be one of allowedTemplates>",
		"historyCleanup":   map[string]any{"preventCleanup": "<boolean>", "keepAllVersionsNewerThanDays": "<int|null>", "keepLatestVersionPerDayForDays": "<int|null>"},
		"collection":       "<map {id} ref to a list view data type, optional>",
		"properties": []any{map[string]any{
			"id":         "<uuid, optional>",
			"name":       "<string, required>",
			"alias":      "<camelCase string, required>",
			"dataType":   map[string]any{"id": "<guid from datatype search>"},
			"dataTypeId": "<guid, accepted shortcut; normalized to dataType.id>",
			"container":  map[string]any{"id": "<container id, optional>"},
			"sortOrder":  "<number>",
		}},
		"containers": []any{map[string]any{
			"id":        "<uuid>",
			"name":      "<tab or group name>",
			"parent":    map[string]any{"id": "<parent container id, optional>"},
			"type":      "<Tab|Group>",
			"sortOrder": "<number>",
		}},
		"compositions": "<array of {documentType: {id}, compositionType: 'Composition'|'Inheritance'} entries, optional>",
	},
	"doctype.update": map[string]any{
		"name":       "<string, required>",
		"alias":      "<camelCase string, required>",
		"properties": "<array; dataTypeId shortcut is accepted on property entries>",
	},
	"datatype.create": map[string]any{
		"id":          "<uuid, optional; generated by CLI when omitted>",
		"name":        "<string, required>",
		"alias":       "<camelCase string, optional>",
		"editorAlias": "<property editor alias, e.g. Umbraco.TextBox>",
		"configuration": map[string]any{
			"<settingAlias>": "<setting value>",
		},
	},
	"media.create": map[string]any{
		"id":        "<uuid, optional; generated by CLI when omitted>",
		"name":      "<string, required>",
		"mediaType": map[string]any{"id": "<media type id>"},
		"parent":    map[string]any{"id": "<parent media id, optional>"},
		"values": []any{map[string]any{
			"alias": "<property alias, e.g. umbracoFile>",
			"value": "<property value or temporary file reference>",
		}},
	},
	"template.create": map[string]any{
		"id":      "<uuid, optional; generated by CLI when omitted>",
		"name":    "<string, required>",
		"alias":   "<camelCase string, required>",
		"content": "<template markup, optional>",
	},
	"member.create": map[string]any{
		"id":         "<uuid, optional; generated by CLI when omitted>",
		"email":      "<string, required, valid email>",
		"username":   "<string, required; usually the email for front-office members>",
		"password":   "<string, required at create time>",
		"memberType": map[string]any{"id": "<member type id, required; from member-type endpoints>"},
		"variants": []any{map[string]any{
			"culture": "<culture, optional; null for invariant member types>",
			"segment": "<segment, optional; null for unsegmented>",
			"name":    "<string, required — the member's display name>",
		}},
		"groups": []any{"<member group GUID from 'member-group list'>"},
		"values": []any{map[string]any{
			"alias":   "<custom property alias defined on the member type>",
			"value":   "<property value>",
			"culture": "<culture, optional; null for invariant>",
			"segment": "<segment, optional; null for unsegmented>",
		}},
		"_apiNote_readOnlyFields": "isApproved, isLockedOut, failedPasswordAttempts, and isTwoFactorEnabled appear on the GET response but are NOT accepted by POST /member or PUT /member/{id} on Umbraco 17.x — they are managed by the auth subsystem. Including them in the payload returns 2xx but the server value does not change.",
	},
}

func buildSchemas() map[string]Schema {
	result := make(map[string]Schema, len(rawSchemas))
	for endpoint, raw := range rawSchemas {
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

func endpointList() []string {
	endpoints := make([]string, 0, len(Schemas))
	for endpoint := range Schemas {
		endpoints = append(endpoints, endpoint)
	}
	sortStrings(endpoints)
	return endpoints
}

func sortStrings(items []string) {
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j] < items[i] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
