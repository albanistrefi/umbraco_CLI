---
name: umbraco-document
description: "Document and content management operations"
metadata:
  version: 0.4.3
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
| `document audit-log <id>` | List the audit trail for a document (who did what, when) |
| `document children <id>` | Get child documents (paginated; --skip/--take/--all) |
| `document domains get <id>` | Get the domains assigned to a document |
| `document get <id>` | Get a document by ID |
| `document public-access get <id>` | Get the public-access (member protection) rules on a document |
| `document publish-descendants-result <id> <task-id>` | Check the progress of an asynchronous publish-descendants run |
| `document referenced-descendants <id>` | List items that reference this document or any of its descendants |
| `document references <id>` | List items that reference this document (paginated; --skip/--take/--all) |
| `document root` | Get root documents (paginated; --skip/--take/--all) |
| `document search` | Search documents |
| `document version get <version-id>` | Get a stored document version (the full payload as it was) |
| `document version list <document-id>` | List stored versions of a document (paginated; --skip/--take/--all) |

### ancestors

```bash
umbraco document ancestors <id>
```

### are-referenced

```bash
umbraco document are-referenced
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | — | Comma-separated document GUIDs to check (required) |

### audit-log

```bash
umbraco document audit-log <id>
```

GET /document/{id}/audit-log. Pass --params for orderDirection or sinceDate filters, e.g. --params '{"sinceDate":"2026-01-01T00:00:00Z"}'.

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

### children

```bash
umbraco document children <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--resolve-doctype` | bool | false | Annotate each item's documentType with its alias (tree responses carry only the id; this fetches each distinct document type once) |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### domains get

```bash
umbraco document domains get <id>
```

### get

```bash
umbraco document get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### public-access get

```bash
umbraco document public-access get <id>
```

### publish-descendants-result

```bash
umbraco document publish-descendants-result <id> <task-id>
```

### referenced-descendants

```bash
umbraco document referenced-descendants <id>
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

### references

```bash
umbraco document references <id>
```

Wraps GET /document/{id}/referenced-by. Used to answer 'what uses this node' for orphan checks, safe-delete verification, and taxonomy usage audits.

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

### root

```bash
umbraco document root
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--resolve-doctype` | bool | false | Annotate each item's documentType with its alias (tree responses carry only the id; this fetches each distinct document type once) |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### search

```bash
umbraco document search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--under` | string | — | Limit search to documents under the given parent ID |

### version get

```bash
umbraco document version get <version-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### version list

```bash
umbraco document version list <document-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--culture` | string | — | Limit versions to one culture on variant content |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `document bulk-update` | Update multiple documents from an explicit ID list |
| `document copy <id>` | Copy a document |
| `document create` | Create a document |
| `document csv-update` | Update multiple documents from a CSV file |
| `document domains set <id>` | Replace the domains assigned to a document |
| `document move <id>` | Move a document |
| `document public-access remove <id>` | Remove the public-access rules from a document (makes it publicly visible again) |
| `document public-access set <id>` | Create or replace the public-access rules on a document |
| `document publish <id>` | Publish a document |
| `document publish-descendants <id>` | Publish a document and its entire subtree |
| `document restore <id>` | Restore a document from the recycle bin |
| `document sort` | Reorder sibling documents |
| `document trash <id>` | Move a document to recycle bin |
| `document unpublish <id>` | Unpublish a document |
| `document update <id>` | Update a document |
| `document update-properties <id>` | Update document properties (merges into values[] by alias) |
| `document version prevent-cleanup <version-id>` | Pin a version so scheduled history cleanup never deletes it |
| `document version rollback <version-id>` | Roll the document back to this version |

### bulk-update

```bash
umbraco document bulk-update
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned requests without executing |
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
| `--dry-run` | bool | false | Print the planned request without executing |
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
| `--dry-run` | bool | false | Print the planned request without executing |
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
| `--dry-run` | bool | false | Print the planned CSV-driven updates without executing them |
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

### domains set

```bash
umbraco document domains set <id>
```

PUT /document/{id}/domains. The PUT replaces the full set: pass every domain that should remain via repeated --domain host=isoCode flags (e.g. --domain example.dk=da-DK), or the raw payload via --json.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--default-iso-code` | string | — | Default culture for unmatched hosts |
| `--domain` | stringArray | [] | Domain assignment as host=isoCode; repeat for multiple |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Domains payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document domains set <id> --dry-run

# 2. Execute
umbraco document domains set <id>
```

### move

```bash
umbraco document move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document move <id> --dry-run

# 2. Execute
umbraco document move <id>
```

### public-access remove

```bash
umbraco document public-access remove <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm removing member protection |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document public-access remove <id> --dry-run

# 2. Execute
umbraco document public-access remove <id>
```

### public-access set

```bash
umbraco document public-access set <id>
```

Sets member protection: which member groups (or named members) may view the document, plus the login and error pages. Payload shape:

  {"loginDocument":{"id":"<guid>"},"errorDocument":{"id":"<guid>"},"memberGroupNames":["Members"],"memberUserNames":[]}

The CLI checks whether rules already exist and issues POST (create) or PUT (replace) accordingly.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Public-access payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document public-access set <id> --dry-run

# 2. Execute
umbraco document public-access set <id>
```

### publish

```bash
umbraco document publish <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture shortcut |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Publish payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document publish <id> --dry-run

# 2. Execute
umbraco document publish <id>
```

### publish-descendants

```bash
umbraco document publish-descendants <id>
```

PUT /document/{id}/publish-with-descendants. Publishes the node and every published-state descendant; pass --include-unpublished to also publish drafts.

On variant content pass --culture per culture to publish; with no --culture the invariant default is used. The operation is asynchronous server-side — the response carries a taskId, and 'document publish-descendants-result <id> <task-id>' reports completion.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | stringArray | [] | Culture to publish; repeat for multiple (omit for invariant content) |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--include-unpublished` | bool | false | Also publish descendants that have never been published |
| `--json` | string | — | Publish payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document publish-descendants <id> --dry-run

# 2. Execute
umbraco document publish-descendants <id>
```

### restore

```bash
umbraco document restore <id>
```

PUT /recycle-bin/document/{id}/restore. The restore target defaults to the document's original parent (looked up via the recycle-bin API); pass --to for a different parent, or --to root to restore at the content root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--to` | string | — | Restore target parent ID, or 'root' (defaults to the original parent) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document restore <id> --dry-run

# 2. Execute
umbraco document restore <id>
```

### sort

```bash
umbraco document sort
```

PUT /document/sort. Pass --ids with the desired order (sortOrder is assigned from position) and --parent for the common parent; omit --parent when sorting root-level documents. IDs not listed keep their relative order after the sorted ones.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated document GUIDs in the desired order |
| `--json` | string | — | Sort payload as JSON |
| `--parent` | string | — | Parent document ID (omit for root-level documents) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document sort --dry-run

# 2. Execute
umbraco document sort
```

### trash

```bash
umbraco document trash <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

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
| `--dry-run` | bool | false | Print the planned request without executing |
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
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current document before update (fields not mentioned are preserved) |
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
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Properties payload as JSON; accepts object {alias: value}, array [{alias, value, culture?, segment?}], or envelope {"values":[...]} |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document update-properties <id> --dry-run

# 2. Execute
umbraco document update-properties <id>
```

### version prevent-cleanup

```bash
umbraco document version prevent-cleanup <version-id>
```

PUT /document-version/{id}/prevent-cleanup. Pins the version by default; pass --disable to unpin it again.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--disable` | bool | false | Allow cleanup to delete this version again |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document version prevent-cleanup <version-id> --dry-run

# 2. Execute
umbraco document version prevent-cleanup <version-id>
```

### version rollback

```bash
umbraco document version rollback <version-id>
```

POST /document-version/{id}/rollback. Version IDs come from 'document version list'. On variant content pass --culture to roll back a single culture; omitting it rolls back the invariant data.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture to roll back on variant content |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco document version rollback <version-id> --dry-run

# 2. Execute
umbraco document version rollback <version-id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco document --help

# Inspect a specific endpoint schema
umbraco schema document.<method>
```
