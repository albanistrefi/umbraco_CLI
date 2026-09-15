---
name: umbraco-mediatype
description: "Media type operations"
metadata:
  version: 0.4.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# mediatype

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco mediatype <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `mediatype children <id>` | Get child media types of a folder (paginated; --skip/--take/--all) |
| `mediatype export <id>` | Export a media type as a .udt document |
| `mediatype get <id-or-alias>` | Get media type by ID (or by exact alias) |
| `mediatype list` | List media types (paginated; --skip/--take/--all) |
| `mediatype search` | Search media types |

### children

```bash
umbraco mediatype children <id>
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

### export

```bash
umbraco mediatype export <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### get

```bash
umbraco mediatype get <id-or-alias>
```

Fetches a media type by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco mediatype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--exclude-folders` | bool | false | Alias for --types-only |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--recursive` | bool | false | Walk media type folders recursively |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--types-only` | bool | false | Return media types only, excluding folders |

### search

```bash
umbraco mediatype search
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
| `mediatype create` | Create a media type |
| `mediatype create-folder` | Create a media type folder (optionally inside another folder) |
| `mediatype delete <id>` | Delete a media type |
| `mediatype delete-folder <id>` | Delete an empty media type folder |
| `mediatype remove-property <id-or-alias>` | Remove a property from a media type by alias |
| `mediatype restore-backup <file>` | Restore a media type from a --backup JSON file |
| `mediatype update <id>` | Update a media type (--json replaces, --merge-json merges) |

### create

```bash
umbraco mediatype create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype create [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype create [flags]
```

### create-folder

```bash
umbraco mediatype create-folder
```

POST /media-type/folder. Creates a folder in the media type tree; --parent nests it inside an existing folder. Pass the folder as --json '{"name": …, "parent": {"id": …}}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). Put a type inside it with 'umbraco mediatype create --json '{..., "parent": {"id": "<folder id>"}}''. After the create the folder is read back, so the result is the persisted record.

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
umbraco mediatype create-folder [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype create-folder [flags]
```

### delete

```bash
umbraco mediatype delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype delete <id> --force [flags]
```

### delete-folder

```bash
umbraco mediatype delete-folder <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype delete-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype delete-folder <id> --force [flags]
```

### remove-property

```bash
umbraco mediatype remove-property <id-or-alias>
```

GET /media-type/{id} + PUT /media-type/{id}. Removes the property with --alias and writes the type back otherwise unchanged; the type is re-read afterwards and the command fails if the property is still there. Content of this type loses the property's values, so the command refuses to run without --dry-run (plan) or --force (confirm). The server prunes tabs/groups left without properties on save — the result lists them under prunedContainers. Pass --backup to save the pre-change type first; 'mediatype restore-backup <file>' puts it back.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Alias of the property to remove (required; exact match) |
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the removal when not using --dry-run |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype remove-property <id-or-alias> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype remove-property <id-or-alias> --force [flags]
```

### restore-backup

```bash
umbraco mediatype restore-backup <file>
```

Reads a file written by any 'mediatype … --backup' command and PUTs the saved media type back to the server, then re-reads it and compares the saved values/properties (alias, culture and segment) and names with what the server now holds; any difference fails the command. The write goes to the id recorded in the envelope; pass --id <guid> to assert which media type the file must belong to before anything is written (the file is refused when it names another id), and --dry-run to see the target without writing. This restores the entity's fields and values; it does not undo a move, publish state, or delete.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Assert the envelope belongs to this id before writing (refuses otherwise) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype restore-backup <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype restore-backup <file> [flags]
```

### update

```bash
umbraco mediatype update <id>
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
umbraco mediatype update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco mediatype --help

# Inspect a specific endpoint schema
umbraco schema mediatype.<method>
```
