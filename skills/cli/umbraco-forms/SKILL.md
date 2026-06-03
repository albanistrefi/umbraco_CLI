---
name: umbraco-forms
description: "Umbraco Forms operations (read-only)"
metadata:
  version: 0.3.13
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
| `forms record <formId> <recordId>` | Get a single form record by its uniqueId (GUID) |
| `forms record-workflow-log <formId> <recordId>` | Get the workflow execution audit trail for a record |
| `forms records <formId>` | List form records (submissions) |

### children

```bash
umbraco forms children <folderId>
```

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

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--scan` | int | 500 | Maximum number of records to scan when looking up the record (the Forms API has no direct GET-by-id, so we filter client-side) |

### record-workflow-log

```bash
umbraco forms record-workflow-log <formId> <recordId>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### records

```bash
umbraco forms records <formId>
```

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
