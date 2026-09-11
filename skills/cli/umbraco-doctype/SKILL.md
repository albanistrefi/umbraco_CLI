---
name: umbraco-doctype
description: "Document type schema operations"
metadata:
  version: 0.4.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# doctype

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco doctype <command> [flags]
```

## Overview

```text
Document type schema operations.

Task → command:
  Read a type by GUID or alias                         doctype get <id-or-alias>
  Find types by name                                   doctype search --query <text>
  Add / reorder properties, add tabs or groups         doctype add-property, reorder-properties, add-container
  Allowed blocks, block order, Block Grid groups       datatype block add|reorder|groups … (blocks belong to the Block List/Grid DATA TYPE, not the document type)
  Which data type does a property use?                 doctype get <id> --fields properties
  Apply a whole schema from Deploy artifacts           deploy apply --uda-dir <dir> --dry-run
```

## Read Commands

| Command | Description |
|---------|-------------|
| `doctype allowed-in-library` | List document types usable as library elements (Umbraco 18.1+) |
| `doctype children <id>` | Get child document types (paginated; --skip/--take/--all) |
| `doctype get <id-or-alias>` | Get document type by ID (or by exact alias) |
| `doctype list` | List document types (paginated; --skip/--take/--all) |
| `doctype root` | Get root document types (paginated; --skip/--take/--all) |
| `doctype search` | Search document types |

### allowed-in-library

```bash
umbraco doctype allowed-in-library
```

GET /document-type/allowed-in-library. Lists the element types with allowedInLibrary set — the types 'element create' accepts.

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
umbraco doctype children <id>
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
umbraco doctype get <id-or-alias>
```

Fetches a document type by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404. Blocks (allowed blocks, their order, Block Grid groups) live on the Block List/Grid data type: see 'umbraco datatype block --help'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco doctype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--exclude-folders` | bool | false | Alias for --types-only |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--recursive` | bool | false | Walk document type folders recursively |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--types-only` | bool | false | Return document types only, excluding folders |

### root

```bash
umbraco doctype root
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

### search

```bash
umbraco doctype search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `doctype add-container <id>` | Append a tab or group container to a document type |
| `doctype add-property <id>` | Append a property to a document type, creating its container with it if needed |
| `doctype copy <id>` | Copy document type |
| `doctype create` | Create document type (pass --element to create an element type) |
| `doctype move <id>` | Move document type |
| `doctype reorder-properties <id>` | Change the order of properties on a document type |
| `doctype update <id>` | Update document type |

### add-container

```bash
umbraco doctype add-container <id>
```

GET /document-type/{id} + PUT /document-type/{id}. Note the server prunes containers that are saved with no properties (verified on 18.1) — this command verifies the container survived the save and errors when it was pruned. To create a container and its first property in one step use 'doctype add-property --create-container'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Display name for the new container |
| `--parent` | string | — | Optional name of an existing parent container (typically a Tab when adding a Group) |
| `--type` | string | — | Container type: Tab or Group |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype add-container <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype add-container <id> [flags]
```

### add-property

```bash
umbraco doctype add-property <id>
```

GET /document-type/{id} + PUT /document-type/{id}. The property lands under the --container tab/group. With --create-container the container is created in the same update when it does not exist yet — the two must travel in one request, because the server prunes containers that are saved empty (which is why a bare 'add-container' followed by 'add-property' cannot work).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Property alias (camelCase identifier) |
| `--container` | string | — | Name of the existing tab/group container that should hold the property (case-insensitive match) |
| `--container-type` | string | Group | Container type for --create-container: Group or Tab |
| `--create-container` | bool | false | Create the --container together with this property when it does not exist yet |
| `--data-type` | string | — | Data type ID (GUID) backing the property |
| `--description` | string | — | Optional property description |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--mandatory` | bool | false | Mark the property as mandatory |
| `--name` | string | — | Human-readable property name |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype add-property <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype add-property <id> [flags]
```

### copy

```bash
umbraco doctype copy <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype copy <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype copy <id> [flags]
```

### create

```bash
umbraco doctype create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--element` | bool | false | Convenience flag for --json '{...,"isElement":true}'; overrides any isElement set in --json |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype create [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype create [flags]
```

### move

```bash
umbraco doctype move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype move <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype move <id> [flags]
```

### reorder-properties

```bash
umbraco doctype reorder-properties <id>
```

GET /document-type/{id} + PUT /document-type/{id}. The Management API has no dedicated reorder operation — property order is the per-container sortOrder field — so this fetches the document type, rewrites sortOrder values, and PUTs the result back. Two modes: --aliases assigns positions 0..n to the listed properties (all in one container) with the container's remaining properties following in their current relative order; --alias with --sort-order sets a single property's sortOrder verbatim (other properties keep theirs, so equal values sort arbitrarily — prefer --aliases for a full deterministic order).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Single property alias to move (requires --sort-order) |
| `--aliases` | string | — | Comma-separated property aliases in the desired order (positions become sortOrder; unlisted properties in the container follow in their current order) |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--sort-order` | int | -1 | Target sortOrder for --alias (0-based) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype reorder-properties <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype reorder-properties <id> [flags]
```

### update

```bash
umbraco doctype update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco doctype --help

# Inspect a specific endpoint schema
umbraco schema doctype.<method>
```
