---
name: umbraco-forms
description: "Umbraco Forms operations: forms, folders, records, prevalue sources"
metadata:
  version: 0.4.25
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# forms

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco forms <command> [flags]
```

## Overview

```text
Commands for the Umbraco Forms Management API (/umbraco/forms/management/api/v1): browse and author forms and folders, act on submitted records (approve, reject, delete, edit, retry workflows), and manage prevalue sources. What the CLI may read or change is governed by the API user's Forms permissions (manage forms, manage workflows, view/edit/delete entries, ...); a refused operation comes back as the server's 403. Every mutation takes --dry-run; deletes and destructive record actions also need --force. Useful for resolving form and field GUIDs when composing Umbraco.Forms.Automate flows.
```

## Read Commands

| Command | Description |
|---------|-------------|
| `forms children <folderId>` | List the forms and sub-folders inside a folder |
| `forms get <id>` | Get form definition by ID (includes fields, pages, workflows) |
| `forms list` | List forms (tree root: returns folders and root-level forms) |
| `forms prevalue-source get <id>` | Get a prevalue source by ID |
| `forms prevalue-source list` | List prevalue sources (paginated; --skip/--take/--all) |
| `forms prevalue-source types` | List prevalue source types (the fieldPreValueSourceTypeId values and their settings) |
| `forms record <formId> <recordId>` | Get a single form record by its uniqueId (GUID); scans the first --scan records (default 500) |
| `forms record-actions` | List the record actions (approve, reject, delete, ...) the API user may run |
| `forms record-workflow-log <formId> <recordId>` | Get the workflow execution audit trail for a record |
| `forms records <formId>` | List form records (submissions) |

### children

```bash
umbraco forms children <folderId>
```

GET /tree/form/children/{folderId}. Forms in Umbraco are organized into folders. 'forms list' returns root-level items (mostly folders); use 'forms children <folderId>' to drill into a folder returned with isFolder=true. Every item carries isFolder and type ("folder" or "form"), so nested folders can be walked. Note: GET /form?folderId=… is not used — the server ignores the filter and returns every form (verified on Forms 17/18). The tree route is not paged either: it returns the whole folder and ignores skip/take (verified: take=2 still returned all 30 items).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |

### get

```bash
umbraco forms get <id>
```

GET /form/{id}. Folders are not forms: a folder id (isFolder=true in 'forms list'/'forms children') is refused with a pointer to 'forms children <folderId>' instead of a bare 404.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### list

```bash
umbraco forms list
```

Returns the Forms tree root. On real installs this is mostly folders. Every item carries isFolder and type ("folder" or "form"); use 'forms children <folderId>' to drill into a folder and 'forms get <formId>' only on forms.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |

### prevalue-source get

```bash
umbraco forms prevalue-source get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### prevalue-source list

```bash
umbraco forms prevalue-source list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### prevalue-source types

```bash
umbraco forms prevalue-source types
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### record

```bash
umbraco forms record <formId> <recordId>
```

Returns one record from a form. recordId is the record's uniqueId (GUID, e.g. 917a242d-d48c-44ac-ad99-9dcfaf2d3e7f), visible in 'forms records' output. The numeric 'id' field is also accepted.

Implementation note: the Forms Management API does not expose a GET endpoint on /form/{formId}/record/{recordId} — only PUT is registered. This subcommand therefore fetches the records list and filters client-side. Use --scan to control how many records are scanned (default 500); for forms with more records, narrow by date with 'forms records --from/--to' and pipe to jq.

Record ordering is controlled by the Forms API and is not part of its public contract. Observation against v17.3 suggests newest-first, but agents shouldn't rely on it — if a record isn't in the scan window, the error distinguishes 'definitely not present' from 'scan window exhausted' so you know whether to widen.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--scan` | int | 500 | Maximum number of records to scan when looking up the record (the Forms API has no direct GET-by-id, so we filter client-side). Must be positive. |

### record-actions

```bash
umbraco forms record-actions
```

GET /record-set-actions. The list is filtered by the API user's Forms permissions: delete appears only when the user may delete entries. Run one with 'forms record-action <formId> <alias> --record-ids …'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### record-workflow-log

```bash
umbraco forms record-workflow-log <formId> <recordId>
```

Returns the per-workflow execution log for a single record. Useful when debugging why an Umbraco.Forms.Automate flow did or did not fire for a given submission.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### records

```bash
umbraco forms records <formId>
```

List records for a form. Filter flags (--state, --from, --to, --skip, --take) are passed through to the Management API verbatim; use --params for any other supported filter.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--from` | string | — | Filter records created on or after this ISO 8601 date/time |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Additional query parameters as JSON; merged with --state/--from/--to/--skip/--take, with --params taking precedence on key collisions |
| `--skip` | int | 0 | Number of records to skip |
| `--state` | string | — | Filter by record state (e.g. submitted, approved, pending). Pass-through; see your Umbraco Forms version for supported values |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 0 | Maximum number of records to return (defaults to 100 if not set; pass --take 0 explicitly for no limit) |
| `--to` | string | — | Filter records created on or before this ISO 8601 date/time |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `forms copy <id>` | Copy a form (optionally with its workflows, into another folder) |
| `forms copy-workflows <sourceFormId>` | Copy workflows from one form onto another |
| `forms create` | Create a form, starting from the server's form scaffold |
| `forms create-folder` | Create a Forms folder (optionally inside another folder) |
| `forms delete <id>` | Permanently delete a form together with its stored records |
| `forms delete-folder <id>` | Permanently delete an empty Forms folder |
| `forms move <id>` | Move a form into another folder (or to the root) |
| `forms move-folder <id>` | Move a Forms folder into another folder (or to the root) |
| `forms prevalue-source create` | Create a prevalue source, starting from the server's scaffold |
| `forms prevalue-source delete <id>` | Permanently delete a prevalue source |
| `forms prevalue-source update <id>` | Update a prevalue source |
| `forms record-action <formId> <action>` | Run a record action (approve, reject, delete, ...) on records of a form |
| `forms record-update <formId> <recordId>` | Change field values on a submitted record |
| `forms record-workflow-retry <formId> <recordId> <workflowId>` | Re-run one workflow for a submitted record |
| `forms update <id>` | Update a form definition (fields, pages, workflows, settings) |
| `forms update-folder <id>` | Rename a Forms folder |

### copy

```bash
umbraco forms copy <id>
```

POST /form/{id}/copy. --name names the copy (the server otherwise appends " (1)" to the source name), --to puts it in a folder (default: the source form's folder), and --copy-workflows carries the source's workflows over. --json sends the raw body instead: {"newName", "copyWorkflows", "copyToFolderId"}. The result carries the new form's id.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--copy-workflows` | bool | false | Copy the source form's workflows too |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Raw copy payload as JSON: {"newName"?, "copyWorkflows", "copyToFolderId"?} |
| `--name` | string | — | Name for the copy (default: the source name with " (1)" appended) |
| `--to` | string | — | Folder GUID to put the copy in (default: the source form's folder) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms copy <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms copy <id> [flags]
```

### copy-workflows

```bash
umbraco forms copy-workflows <sourceFormId>
```

POST /form/{sourceFormId}/copy-workflows. --workflow-ids names the source form's workflows to copy (ids from 'forms get <sourceFormId> --fields formWorkflows'); --to is the destination form. The copies are added to the destination's existing workflows. --json sends the raw body instead: {"destinationId", "workflowIds": [...]}.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Raw payload as JSON: {"destinationId", "workflowIds": [...]} |
| `--to` | string | — | Destination form GUID |
| `--workflow-ids` | string | — | Comma-separated GUIDs of the source form's workflows to copy |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms copy-workflows <sourceFormId> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms copy-workflows <sourceFormId> [flags]
```

### create

```bash
umbraco forms create
```

POST /form. The full form model has some thirty required fields, so the CLI fetches GET /form/scaffold (the same starting point the backoffice uses: a fresh id, the default page/field layout, and the install's default workflows) and deep-merges --json on top; only what differs from the scaffold needs naming, and "name" is required. Put the form in a folder with {"folderId": "<folder id>"} (folders from 'forms list'/'forms children', isFolder=true; omit for the root). The scaffold's default workflows (Umbraco:Forms:Options:DefaultWorkflows, e.g. a notification email) are kept unless --json replaces them, e.g. {"formWorkflows": {"onSubmit": [], "onApprove": [], "onReject": []}}; pages and other arrays are replaced wholesale, not merged. The result carries the new form's id; read it back with 'forms get <id>'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Form fields as JSON, deep-merged onto GET /form/scaffold; must name "name" |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms create [flags] --dry-run

# 2. Execute with the same flags
umbraco forms create [flags]
```

### create-folder

```bash
umbraco forms create-folder
```

POST /folder. Pass the folder as --json '{"name": …, "parentId": …}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). Put a form inside it with 'forms create --json '{"name": …, "folderId": "<folder id>"}'' or 'forms move <id> --to <folder id>'. After the create the folder is read back, so the result is the persisted record.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Folder GUID to use (generated when omitted) |
| `--json` | string | — | Folder payload as JSON: {"id"?, "name", "parentId"?} |
| `--name` | string | — | Folder name (fills name when --json omits it) |
| `--parent` | string | — | Parent folder GUID; omit for a root-level folder |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms create-folder [flags] --dry-run

# 2. Execute with the same flags
umbraco forms create-folder [flags]
```

### delete

```bash
umbraco forms delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms delete <id> --force [flags]
```

### delete-folder

```bash
umbraco forms delete-folder <id>
```

DELETE /folder/{id}. Only empty folders can be deleted: the CLI checks GET /folder/{id}/is-empty first and refuses a folder that still holds forms or sub-folders (and an id that is no folder, which is-empty reports as non-empty), because the server answers that case with a bare 500 (a database constraint error) rather than a validation message (verified on Forms 18.1). Move or delete the contents first.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms delete-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms delete-folder <id> --force [flags]
```

### move

```bash
umbraco forms move <id>
```

PUT /form/{id}/move. --to takes a folder id (isFolder=true in 'forms list'/'forms children'); --to-root moves the form out of every folder.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Raw move payload as JSON: {"parentId": "<folder id>" | null} |
| `--to` | string | — | Target folder GUID |
| `--to-root` | bool | false | Move to the root of the Forms tree |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms move <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms move <id> [flags]
```

### move-folder

```bash
umbraco forms move-folder <id>
```

PUT /folder/{id}/move. The folder's forms and sub-folders move with it.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Raw move payload as JSON: {"parentId": "<folder id>" | null} |
| `--to` | string | — | Target folder GUID |
| `--to-root` | bool | false | Move to the root of the Forms tree |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms move-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms move-folder <id> [flags]
```

### prevalue-source create

```bash
umbraco forms prevalue-source create
```

POST /prevalue-source. The CLI fetches GET /prevalue-source/scaffold (a fresh id and the required defaults) and deep-merges --json on top. Required: "name" and "fieldPreValueSourceTypeId" (an id from 'forms prevalue-source types'), plus that type's "settings" (setting aliases are listed per type).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Prevalue source fields as JSON, deep-merged onto GET /prevalue-source/scaffold |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms prevalue-source create [flags] --dry-run

# 2. Execute with the same flags
umbraco forms prevalue-source create [flags]
```

### prevalue-source delete

```bash
umbraco forms prevalue-source delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms prevalue-source delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms prevalue-source delete <id> --force [flags]
```

### prevalue-source update

```bash
umbraco forms prevalue-source update <id>
```

PUT /prevalue-source/{id}. --merge-json fetches the source and deep-merges the patch (the safe default); --json replaces it wholesale. The body's id is filled in from the argument and a mismatching one is refused.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms prevalue-source update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms prevalue-source update <id> [flags]
```

### record-action

```bash
umbraco forms record-action <formId> <action>
```

POST /form/{formId}/record/actions/{actionId}/execute. <action> is an alias, id or name from 'forms record-actions' (e.g. approve, reject). --record-ids takes record uniqueIds (GUIDs) from 'forms records <formId> --fields uniqueId,state'. Delete actions, and any action the backoffice asks to confirm, permanently change records and need --force (or --dry-run to rehearse). The server ignores record ids it does not know and still answers 200 (verified on Forms 18.1), so read the records back with 'forms records' to confirm the new state.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm a destructive action (delete) |
| `--record-ids` | string | — | Comma-separated record uniqueIds (GUIDs) to run the action on (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms record-action <formId> <action> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms record-action <formId> <action> --force [flags]
```

### record-update

```bash
umbraco forms record-update <formId> <recordId>
```

PUT /form/{formId}/record/{recordId}. --json is an array with one entry per field to change: [{"fieldId": "<field GUID>", "values": ["new value"]}]. recordId is the record's uniqueId (GUID). Needs the Forms edit-entries permission. Record contents are not echoed back; read the record with 'forms record <formId> <recordId>'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Field changes as a JSON array: [{"fieldId", "values": [...]}] (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms record-update <formId> <recordId> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms record-update <formId> <recordId> [flags]
```

### record-workflow-retry

```bash
umbraco forms record-workflow-retry <formId> <recordId> <workflowId>
```

POST /form/{formId}/record/{recordId}/workflow/{workflowId}/retry. Use after 'forms record-workflow-log <formId> <recordId>' shows a failed workflow. The workflow really runs again, with its side effects (emails are re-sent, data is re-posted), so rehearse with --dry-run first.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms record-workflow-retry <formId> <recordId> <workflowId> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms record-workflow-retry <formId> <recordId> <workflowId> [flags]
```

### update

```bash
umbraco forms update <id>
```

PUT /form/{id}. --merge-json fetches the form and deep-merges the patch, so unmentioned settings survive — the safe default (e.g. --merge-json '{"name": "New name"}'). --json is a full replacement: every field it leaves out is reset to its default, including folderId (the form moves to the root) and pages (the form loses its fields) — start from 'forms get <id>' output when using it. Arrays such as pages and formWorkflows.onSubmit are replaced wholesale by a patch, not merged entry by entry. The body's id is filled in from the argument — a PUT whose body has no id leaves the form untouched and creates a new one at the root instead — and a mismatching one is refused. Workflows live on the form: edit formWorkflows here, or copy them from another form with 'forms copy-workflows'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms update <id> [flags]
```

### update-folder

```bash
umbraco forms update-folder <id>
```

PUT /folder/{id}. The update model carries only the name, e.g. --merge-json '{"name": "New name"}' or --json '{"name": "New name"}'. Moving a folder is 'forms move-folder'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco forms update-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco forms update-folder <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco forms --help

# Inspect a specific endpoint schema
umbraco schema forms.<method>
```
