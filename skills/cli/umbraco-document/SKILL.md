---
name: umbraco-document
description: "Document and content management operations"
metadata:
  version: 0.3.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# document

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco document <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `document ancestors <id>` | Get ancestor documents |
| `document are-referenced` | Bulk check: which of these document IDs are referenced by something |
| `document children <id>` | Get child documents (paginated; --skip/--take/--all) |
| `document get <id>` | Get a document by ID |
| `document referenced-descendants <id>` | List items that reference this document or any of its descendants |
| `document references <id>` | List items that reference this document (paginated; --skip/--take/--all) |
| `document root` | Get root documents (paginated; --skip/--take/--all) |
| `document search` | Search documents |

### ancestors

```bash
umbraco document ancestors <id>
```

### are-referenced

```bash
umbraco document are-referenced
```

GET /document/are-referenced?id=<csv>. Returns the ids that have at least one inbound reference; safe-to-delete candidates are the complement.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | — | Comma-separated document GUIDs to check (required) |

### children

```bash
umbraco document children <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### get

```bash
umbraco document get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### referenced-descendants

```bash
umbraco document referenced-descendants <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### references

```bash
umbraco document references <id>
```

Wraps GET /document/{id}/referenced-by. Used to answer 'what uses this node' for orphan checks, safe-delete verification, and taxonomy usage audits.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### root

```bash
umbraco document root
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### search

```bash
umbraco document search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON |
| `--query` | string | — | Search query (convenience) |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |
| `--under` | string | — | Limit search to documents under the given parent ID |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `document bulk-update` | Update multiple documents from an explicit ID list |
| `document copy <id>` | Copy a document |
| `document create` | Create a document |
| `document csv-update` | Update multiple documents from a CSV file |
| `document move <id>` | Move a document |
| `document publish <id>` | Publish a document |
| `document restore <id>` | Restore a document |
| `document trash <id>` | Move a document to recycle bin |
| `document unpublish <id>` | Unpublish a document |
| `document update <id>` | Update a document |
| `document update-properties <id>` | Update document properties (merges into values[] by alias) |

### bulk-update

```bash
umbraco document bulk-update
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate requests without executing |
| `--force` | bool | false | Confirm the bulk update when not using --dry-run |
| `--id` | stringArray | [] | Document ID to update; repeat for multiple documents |
| `--id-file` | string | — | Path to a file containing document IDs, one per line |
| `--json` | string | — | Full JSON payload applied to every document |
| `--merge-json` | string | — | Partial JSON payload merged into each current document before update |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document bulk-update --dry-run

# 2. Execute
umbraco document bulk-update
```

### copy

```bash
umbraco document copy <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture shortcut for --publish |
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Copy payload as JSON |
| `--publish` | bool | false | Publish the copied document after a successful copy |
| `--to` | string | — | Target parent ID shortcut |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document copy <id> --dry-run

# 2. Execute
umbraco document copy <id>
```

### create

```bash
umbraco document create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Full JSON payload |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document create --dry-run

# 2. Execute
umbraco document create
```

### csv-update

```bash
umbraco document csv-update
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate the CSV-driven updates without executing them |
| `--field` | stringArray | [] | Explicit alias=column CSV mapping; repeat for multiple properties |
| `--file` | string | — | Path to the CSV file |
| `--force` | bool | false | Confirm the CSV-driven updates when not using --dry-run |
| `--id-column` | string | id | CSV column containing document IDs |
| `--property` | stringArray | [] | Property alias to update from a CSV column with the same name; repeat for multiple properties |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document csv-update --dry-run

# 2. Execute
umbraco document csv-update
```

### move

```bash
umbraco document move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Move payload as JSON |
| `--to` | string | — | Target parent ID shortcut |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document move <id> --dry-run

# 2. Execute
umbraco document move <id>
```

### publish

```bash
umbraco document publish <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture shortcut |
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Publish payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document publish <id> --dry-run

# 2. Execute
umbraco document publish <id>
```

### restore

```bash
umbraco document restore <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document restore <id> --dry-run

# 2. Execute
umbraco document restore <id>
```

### trash

```bash
umbraco document trash <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document trash <id> --dry-run

# 2. Execute
umbraco document trash <id>
```

### unpublish

```bash
umbraco document unpublish <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture shortcut |
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Unpublish payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document unpublish <id> --dry-run

# 2. Execute
umbraco document unpublish <id>
```

### update

```bash
umbraco document update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture shortcut for --save-and-publish |
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON |
| `--merge-json` | string | — | Partial JSON payload merged into the current document before update |
| `--property` | string | — | Update a single property alias without constructing the full payload |
| `--save-and-publish` | bool | false | Publish the document after a successful update |
| `--value` | string | — | String value used with --property |
| `--value-json` | string | — | JSON value used with --property |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document update <id> --dry-run

# 2. Execute
umbraco document update <id>
```

### update-properties

```bash
umbraco document update-properties <id>
```

Updates one or more property values on a document by merging into its values[] array.

Three input shapes are accepted:

  Object form (most common for invariant docs):
    --json '{"isFeatured": true, "products": ["Umbraco CMS"]}'
    Each key becomes a values[] entry with culture=null, segment=null.

  Array form (for culture/segment-variant properties):
    --json '[{"alias":"title","value":"Hi","culture":"en-US","segment":null}]'
    Used verbatim as values[].

  Envelope form (matches 'document update --merge-json'):
    --json '{"values":[{"alias":"title","value":"Hi","culture":null,"segment":null}]}'

In all shapes the resulting values[] is merged by alias into the current document, so untouched properties survive.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Properties payload as JSON; accepts object {alias: value}, array [{alias, value, culture?, segment?}], or envelope {"values":[...]} |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document update-properties <id> --dry-run

# 2. Execute
umbraco document update-properties <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco document --help

# Inspect a specific endpoint schema
umbraco schema document.<method>
```
