---
name: umbraco-forms
description: "Umbraco Forms operations (read-only)"
metadata:
  version: 0.4.4
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

## Read Commands

| Command | Description |
|---------|-------------|
| `forms children <folderId>` | List forms inside a folder |
| `forms get <id>` | Get form definition by ID (includes fields, pages, workflows) |
| `forms list` | List forms (tree root: returns folders and root-level forms) |
| `forms record <formId> <recordId>` | Get a single form record by its uniqueId (GUID); scans the first --scan records (default 500) |
| `forms record-workflow-log <formId> <recordId>` | Get the workflow execution audit trail for a record |
| `forms records <formId>` | List form records (submissions) |

### children

```bash
umbraco forms children <folderId>
```

Forms in Umbraco are organized into folders. 'forms list' returns root-level items (mostly folders); use 'forms children <folderId>' to drill into a folder returned with isFolder=true.

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

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### list

```bash
umbraco forms list
```

Returns the Forms tree root. On real installs this is mostly folders — use 'forms children <folderId>' to drill into a folder returned with isFolder=true.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |

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

## Discovering Commands

```bash
# Browse subcommands
umbraco forms --help

# Inspect a specific endpoint schema
umbraco schema forms.<method>
```
