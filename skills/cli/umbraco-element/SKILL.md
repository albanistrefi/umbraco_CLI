---
name: umbraco-element
description: "Element library content (Umbraco 18.1+): reusable content items with publish lifecycle"
metadata:
  version: 0.4.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# element

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco element <command> [flags]
```

## Overview

```text
Manages elements — reusable content items introduced in Umbraco 18.1 that live in a folder library rather than the page tree. Element types are document types with allowedInLibrary set ('doctype allowed-in-library' lists them). Requires an 18.1+ server.
```

## Read Commands

| Command | Description |
|---------|-------------|
| `element ancestors <id>` | Get ancestor folders of an element |
| `element are-referenced` | Bulk check: which of these element IDs are referenced by something |
| `element audit-log <id>` | List the audit trail for an element (who did what, when) |
| `element bin children <id>` | List children of a trashed element item |
| `element bin list` | List element items at the recycle bin root |
| `element bin original-parent <id>` | Get the original parent of a trashed element item (the default restore target) |
| `element children <parent-id>` | List children of a library folder (paginated; --skip/--take/--all) |
| `element get <id>` | Get an element by ID |
| `element list` | List elements and folders at the library root (paginated; --skip/--take/--all) |
| `element published <id>` | Get the published snapshot of an element |
| `element referenced-descendants <id>` | List items that reference this element folder or anything inside it |
| `element references <id>` | List items that reference this element (paginated; --skip/--take/--all) |
| `element search` | Search elements |
| `element version get <version-id>` | Get a stored element version (the full payload as it was) |
| `element version list <element-id>` | List stored versions of an element (paginated; --skip/--take/--all) |

### ancestors

```bash
umbraco element ancestors <id>
```

### are-referenced

```bash
umbraco element are-referenced
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | — | Comma-separated element GUIDs to check (required) |

### audit-log

```bash
umbraco element audit-log <id>
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

### bin children

```bash
umbraco element bin children <id>
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

### bin list

```bash
umbraco element bin list
```

GET /recycle-bin/element/root. Paginated; use 'bin children <id>' to descend into trashed subtrees.

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

### bin original-parent

```bash
umbraco element bin original-parent <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### children

```bash
umbraco element children <parent-id>
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

### get

```bash
umbraco element get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco element list
```

GET /tree/element/root. Use 'element children <id>' to descend into folders.

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

### published

```bash
umbraco element published <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### referenced-descendants

```bash
umbraco element referenced-descendants <id>
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
umbraco element references <id>
```

Wraps GET /element/{id}/referenced-by. Answers 'what uses this element' before unpublishing or deleting it.

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

### search

```bash
umbraco element search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### version get

```bash
umbraco element version get <version-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### version list

```bash
umbraco element version list <element-id>
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
| `element bin delete <id>` | Permanently delete one element item from the recycle bin |
| `element bin empty` | Permanently delete everything in the element recycle bin |
| `element copy <id>` | Copy an element |
| `element create` | Create an element |
| `element delete <id>` | Permanently delete an element (use 'trash' for the recycle bin) |
| `element move <id>` | Move an element |
| `element publish <id>` | Publish an element |
| `element restore <id>` | Restore a element item from the recycle bin |
| `element trash <id>` | Move an element to the recycle bin |
| `element unpublish <id>` | Unpublish an element |
| `element update <id>` | Update an element |
| `element version prevent-cleanup <version-id>` | Pin a version so scheduled history cleanup never deletes it |
| `element version rollback <version-id>` | Roll the element back to this version |

### bin delete

```bash
umbraco element bin delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element bin delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element bin delete <id> --force [flags]
```

### bin empty

```bash
umbraco element bin empty
```

DELETE /recycle-bin/element. Destroys every trashed element item; there is no undo.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm emptying the recycle bin |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element bin empty [flags] --dry-run

# 2. Execute with the same flags
umbraco element bin empty --force [flags]
```

### copy

```bash
umbraco element copy <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element copy <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element copy <id> [flags]
```

### create

```bash
umbraco element create
```

POST /element, or POST /element/create-and-publish with --publish: created and published in one atomic server-side operation. Required payload fields: documentType ({"id":...} of a type with allowedInLibrary), values, variants; parent ({"id":...} of a library folder) is optional.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Comma-separated cultures to publish with --publish; omit for invariant content |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full JSON payload |
| `--publish` | bool | false | Create and publish atomically via POST /element/create-and-publish |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element create [flags] --dry-run

# 2. Execute with the same flags
umbraco element create [flags]
```

### delete

```bash
umbraco element delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element delete <id> --force [flags]
```

### move

```bash
umbraco element move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element move <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element move <id> [flags]
```

### publish

```bash
umbraco element publish <id>
```

PUT /element/{id}/publish. Defaults to the invariant publish schedule; pass --culture for one culture or --json for a full publishSchedules payload.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture to publish on variant content |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Publish payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element publish <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element publish <id> [flags]
```

### restore

```bash
umbraco element restore <id>
```

PUT /recycle-bin/element/{id}/restore. The restore target defaults to the item's original parent (looked up via the recycle-bin API); pass --to for a different parent, or --to root to restore at the library root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--to` | string | — | Restore target parent ID, or 'root' (defaults to the original parent) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element restore <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element restore <id> [flags]
```

### trash

```bash
umbraco element trash <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element trash <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element trash <id> [flags]
```

### unpublish

```bash
umbraco element unpublish <id>
```

PUT /element/{id}/unpublish. Unpublishes all cultures by default; pass --culture for one culture or --json for a full cultures payload.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture to unpublish on variant content |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Unpublish payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element unpublish <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element unpublish <id> [flags]
```

### update

```bash
umbraco element update <id>
```

PUT /element/{id}, or PUT /element/{id}/update-and-publish with --save-and-publish (one atomic operation; --culture names the cultures to publish, omit for invariant content).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Comma-separated cultures to publish with --save-and-publish; omit for invariant content |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current element before update (fields not mentioned are preserved) |
| `--save-and-publish` | bool | false | Publish atomically with the update via PUT /element/{id}/update-and-publish |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element update <id> [flags]
```

### version prevent-cleanup

```bash
umbraco element version prevent-cleanup <version-id>
```

PUT /element-version/{id}/prevent-cleanup. Pins the version by default; pass --disable to unpin it again.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--disable` | bool | false | Allow cleanup to delete this version again |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element version prevent-cleanup <version-id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element version prevent-cleanup <version-id> [flags]
```

### version rollback

```bash
umbraco element version rollback <version-id>
```

POST /element-version/{id}/rollback. Version IDs come from 'element version list'. On variant content pass --culture to roll back a single culture; omitting it rolls back the invariant data.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture to roll back on variant content |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco element version rollback <version-id> [flags] --dry-run

# 2. Execute with the same flags
umbraco element version rollback <version-id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco element --help

# Inspect a specific endpoint schema
umbraco schema element.<method>
```
