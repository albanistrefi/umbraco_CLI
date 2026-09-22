---
name: umbraco-doctype
description: "Document type schema operations"
metadata:
  version: 0.4.21
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
  Add / reorder / remove properties, add tabs/groups   doctype add-property, reorder-properties, remove-property --alias <a> --backup --force, add-container
  Create a folder, put a type in a folder              doctype create-folder --name <n> [--parent <id>]; doctype create --json '{..."parent":{"id":…}}' or doctype move <id> --to <folder>
  Allowed blocks, block order, Block Grid groups       datatype block add|reorder|groups … (blocks belong to the Block List/Grid DATA TYPE, not the document type)
  Which data type does a property use?                 doctype get <id> --fields properties
  Is a property actually filled in anywhere?           doctype property-is-used <id-or-alias> --alias <propertyAlias>
  Apply a whole schema from Deploy artifacts           deploy apply --uda-dir <dir> --dry-run
```

## Read Commands

| Command | Description |
|---------|-------------|
| `doctype allowed-in-library` | List document types usable as library elements (Umbraco 18.1+) |
| `doctype children <id>` | Get child document types (paginated; --skip/--take/--all) |
| `doctype get <id-or-alias>` | Get document type by ID (or by exact alias) |
| `doctype list` | List document types (paginated; --skip/--take/--all) |
| `doctype property-is-used <id-or-alias>` | Check whether a property of this document type holds a value anywhere |
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

### property-is-used

```bash
umbraco doctype property-is-used <id-or-alias>
```

GET /property-type/is-used?contentTypeId=&propertyAlias=. The document type is addressed by GUID or by exact alias; --alias names the property on it. The response is a bare boolean: true means removing the property would discard stored values.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Property alias on the document type (required) |

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
| `doctype create-folder` | Create a document type folder (optionally inside another folder) |
| `doctype delete-folder <id>` | Delete an empty document type folder |
| `doctype move <id>` | Move document type |
| `doctype remove-property <id-or-alias>` | Remove a property from a document type by alias |
| `doctype reorder-properties <id>` | Change the order of properties on a document type |
| `doctype restore-backup <file>` | Restore a document type from a --backup JSON file |
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

### create-folder

```bash
umbraco doctype create-folder
```

POST /document-type/folder. Creates a folder in the document type tree; --parent nests it inside an existing folder. Pass the folder as --json '{"name": …, "parent": {"id": …}}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). Put a type inside it with 'umbraco doctype create --json '{..., "parent": {"id": "<folder id>"}}'' or 'umbraco doctype move <id> --to <folder id>'. After the create the folder is read back, so the result is the persisted record.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Folder GUID to use (generated when omitted) |
| `--json` | string | — | Folder payload as JSON: {"id"?, "name", "parent"?: {"id"}} |
| `--name` | string | — | Folder name (fills name when --json omits it) |
| `--parent` | string | — | Parent folder GUID; omit for a root-level folder |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype create-folder [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype create-folder [flags]
```

### delete-folder

```bash
umbraco doctype delete-folder <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype delete-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype delete-folder <id> --force [flags]
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

### remove-property

```bash
umbraco doctype remove-property <id-or-alias>
```

GET /document-type/{id} + PUT /document-type/{id}. Removes the property with --alias and writes the type back otherwise unchanged; the type is re-read afterwards and the command fails if the property is still there. Content of this type loses the property's values, so the command refuses to run without --dry-run (plan) or --force (confirm). The server prunes tabs/groups left without properties on save — the result lists them under prunedContainers. Pass --backup to save the pre-change type first; 'doctype restore-backup <file>' puts it back.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Alias of the property to remove (required; exact match) |
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the removal when not using --dry-run |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype remove-property <id-or-alias> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype remove-property <id-or-alias> --force [flags]
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

### restore-backup

```bash
umbraco doctype restore-backup <file>
```

Reads a file written by any 'doctype … --backup' command and PUTs the saved document type back to the server, then re-reads it and compares the saved values/properties (alias, culture and segment) and names with what the server now holds; any difference fails the command. The write goes to the id recorded in the envelope; pass --id <guid> to assert which document type the file must belong to before anything is written (the file is refused when it names another id), and --dry-run to see the target without writing. This restores the entity's fields and values; it does not undo a move, publish state, or delete.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Assert the envelope belongs to this id before writing (refuses otherwise) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco doctype restore-backup <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco doctype restore-backup <file> [flags]
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
