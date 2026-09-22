---
name: umbraco-blueprint
description: "Document blueprints: reusable content presets for new documents"
metadata:
  version: 0.4.23
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# blueprint

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco blueprint <command> [flags]
```

## Overview

```text
Document blueprints: reusable content presets for new documents.

A blueprint is a saved set of property values for one document type. The
backoffice offers it when an editor creates a document; from the CLI the
same thing is 'document create --from-blueprint <id>'.

Task → command:
  Browse the blueprint tree                            blueprint list; blueprint children <folder-id>
  Locate one in the tree                               blueprint ancestors <id>; blueprint siblings <id>
  Read one blueprint and its values                    blueprint get <id>
  Resolve several GUIDs to names in one call           blueprint items --ids <a,b>
  See who changed one, and when                        blueprint audit-log <id>
  Turn an existing document into a blueprint           blueprint create-from-document <document-id> --name <n> [--parent <folder-id>]
  Build a blueprint from scratch                       blueprint create --print-template, then blueprint create --json '{...}'
  Change the stored values                             blueprint update <id> --merge-json '{...}' --backup
  Organize the tree                                    blueprint create-folder --name <n> [--parent <id>]; blueprint move <id> --to <folder-id>
  Inspect the document payload it produces             blueprint scaffold <id>
  Create a document from it                            document create --from-blueprint <id> --parent <doc-id>
  Remove one                                           blueprint delete <id> --force; blueprint delete-folder <id> --force
```

## Read Commands

| Command | Description |
|---------|-------------|
| `blueprint ancestors <id>` | List the folders a blueprint sits under, root first |
| `blueprint audit-log <id>` | List the audit trail for a blueprint (who did what, when) |
| `blueprint children <parent-id>` | List blueprints inside a folder (paginated; --skip/--take/--all) |
| `blueprint get <id>` | Get a blueprint by ID |
| `blueprint items` | Resolve blueprint GUIDs to names in one call |
| `blueprint list` | List blueprints and folders at the tree root (paginated; --skip/--take/--all) |
| `blueprint scaffold <id>` | Print the document skeleton a blueprint produces |
| `blueprint siblings <id>` | List the tree entries around a blueprint or folder |

### ancestors

```bash
umbraco blueprint ancestors <id>
```

GET /tree/document-blueprint/ancestors?descendantId={id}. Answers with the folder chain above the blueprint (or folder), root first and ending with the entry itself, so an id seen in a payload can be placed in the tree without walking it from the root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### audit-log

```bash
umbraco blueprint audit-log <id>
```

GET /document-blueprint/{id}/audit-log. Pass --params for orderDirection or sinceDate filters, e.g. --params '{"sinceDate":"2026-01-01T00:00:00Z"}'.

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
umbraco blueprint children <parent-id>
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
umbraco blueprint get <id>
```

GET /document-blueprint/{id}. The response carries documentType, values and variants; the blueprint's name lives on variants[].name, not at the top level.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### items

```bash
umbraco blueprint items
```

GET /item/document-blueprint?id=…. The item read for blueprints: pass the GUIDs seen in other payloads and get their names and document types back without one request per ID.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--ids` | string | — | Comma-separated blueprint GUIDs (required) |

### list

```bash
umbraco blueprint list
```

GET /tree/document-blueprint/root. Folders come back with isFolder true; use 'blueprint children <id>' to descend into one.

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

### scaffold

```bash
umbraco blueprint scaffold <id>
```

GET /document-blueprint/{id}/scaffold. Returns the blueprint's values and variants as the server would seed a new document with them. 'document create --from-blueprint <id>' consumes this directly.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### siblings

```bash
umbraco blueprint siblings <id>
```

GET /tree/document-blueprint/siblings?target={id}. The window is counted from the target: --before entries above it and --after below it. The response carries totalBefore/totalAfter alongside items, so a wider window is only worth asking for when those are non-zero.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--after` | int | 10 | How many siblings below the target to return |
| `--before` | int | 10 | How many siblings above the target to return |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--folders-only` | bool | false | Return only folders, skipping the blueprints themselves |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `blueprint create` | Create a blueprint from a full JSON payload |
| `blueprint create-folder` | Create a document blueprint folder (optionally inside another folder) |
| `blueprint create-from-document <document-id>` | Capture an existing document as a blueprint |
| `blueprint delete <id>` | Permanently delete a blueprint |
| `blueprint delete-folder <id>` | Delete an empty document blueprint folder |
| `blueprint move <id>` | Move a blueprint into another folder |
| `blueprint update <id>` | Update a blueprint's stored values |

### create

```bash
umbraco blueprint create
```

POST /document-blueprint. Required payload fields: documentType ({"id":…}), values, variants (variants[].name is the blueprint name); parent ({"id":…} of a blueprint folder) is optional. To capture an existing document instead, use 'blueprint create-from-document'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full JSON payload |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint create [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint create [flags]
```

### create-folder

```bash
umbraco blueprint create-folder
```

POST /document-blueprint/folder. Creates a folder in the document blueprint tree; --parent nests it inside an existing folder. Pass the folder as --json '{"name": …, "parent": {"id": …}}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). Put a type inside it with 'umbraco blueprint create --json '{..., "parent": {"id": "<folder id>"}}'' or 'umbraco blueprint move <id> --to <folder id>'. After the create the folder is read back, so the result is the persisted record.

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
umbraco blueprint create-folder [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint create-folder [flags]
```

### create-from-document

```bash
umbraco blueprint create-from-document <document-id>
```

POST /document-blueprint/from-document. Copies the document's current property values into a new blueprint; --name is the blueprint's name (it does not have to match the document) and --parent nests it in a blueprint folder.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Blueprint GUID to use (generated when omitted) |
| `--name` | string | — | Blueprint name (required) |
| `--parent` | string | — | Blueprint folder GUID; omit for the tree root |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint create-from-document <document-id> [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint create-from-document <document-id> [flags]
```

### delete

```bash
umbraco blueprint delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint delete <id> --force [flags]
```

### delete-folder

```bash
umbraco blueprint delete-folder <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint delete-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint delete-folder <id> --force [flags]
```

### move

```bash
umbraco blueprint move <id>
```

PUT /document-blueprint/{id}/move. --to takes the destination folder GUID; --json '{"target":null}' moves the blueprint back to the tree root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint move <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint move <id> [flags]
```

### update

```bash
umbraco blueprint update <id>
```

PUT /document-blueprint/{id}. The update model takes values and variants; --merge-json keeps the rest of the blueprint as it is, --json replaces it wholesale. Rename a blueprint by patching variants[].name.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco blueprint update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco blueprint update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco blueprint --help

# Inspect a specific endpoint schema
umbraco schema blueprint.<method>
```
