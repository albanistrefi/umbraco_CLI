---
name: umbraco-membertype
description: "Member type operations"
metadata:
  version: 0.4.21
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# membertype

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco membertype <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `membertype children <id>` | Get child member types of a folder (paginated; --skip/--take/--all) |
| `membertype export <id>` | Export a member type as a .udt document |
| `membertype get <id-or-alias>` | Get member type by ID (or by exact alias) |
| `membertype list` | List member types (paginated; --skip/--take/--all) |
| `membertype search` | Search member types |

### children

```bash
umbraco membertype children <id>
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
umbraco membertype export <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### get

```bash
umbraco membertype get <id-or-alias>
```

Fetches a member type by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco membertype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--exclude-folders` | bool | false | Alias for --types-only |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--recursive` | bool | false | Walk member type folders recursively |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--types-only` | bool | false | Return member types only, excluding folders |

### search

```bash
umbraco membertype search
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
| `membertype create` | Create a member type |
| `membertype create-folder` | Create a member type folder (optionally inside another folder) |
| `membertype delete <id>` | Delete a member type |
| `membertype delete-folder <id>` | Delete an empty member type folder |
| `membertype remove-property <id-or-alias>` | Remove a property from a member type by alias |
| `membertype restore-backup <file>` | Restore a member type from a --backup JSON file |
| `membertype update <id>` | Update a member type (--json replaces, --merge-json merges) |

### create

```bash
umbraco membertype create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco membertype create [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype create [flags]
```

### create-folder

```bash
umbraco membertype create-folder
```

POST /member-type/folder. Creates a folder in the member type tree; --parent nests it inside an existing folder. Pass the folder as --json '{"name": …, "parent": {"id": …}}' or through --name/--parent (flags fill fields the payload omits; the id is generated when neither supplies one). Put a type inside it with 'umbraco membertype create --json '{..., "parent": {"id": "<folder id>"}}''. After the create the folder is read back, so the result is the persisted record.

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
umbraco membertype create-folder [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype create-folder [flags]
```

### delete

```bash
umbraco membertype delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco membertype delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype delete <id> --force [flags]
```

### delete-folder

```bash
umbraco membertype delete-folder <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco membertype delete-folder <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype delete-folder <id> --force [flags]
```

### remove-property

```bash
umbraco membertype remove-property <id-or-alias>
```

GET /member-type/{id} + PUT /member-type/{id}. Removes the property with --alias and writes the type back otherwise unchanged; the type is re-read afterwards and the command fails if the property is still there. Content of this type loses the property's values, so the command refuses to run without --dry-run (plan) or --force (confirm). The server prunes tabs/groups left without properties on save — the result lists them under prunedContainers. Pass --backup to save the pre-change type first; 'membertype restore-backup <file>' puts it back.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Alias of the property to remove (required; exact match) |
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the removal when not using --dry-run |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco membertype remove-property <id-or-alias> [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype remove-property <id-or-alias> --force [flags]
```

### restore-backup

```bash
umbraco membertype restore-backup <file>
```

Reads a file written by any 'membertype … --backup' command and PUTs the saved member type back to the server, then re-reads it and compares the saved values/properties (alias, culture and segment) and names with what the server now holds; any difference fails the command. The write goes to the id recorded in the envelope; pass --id <guid> to assert which member type the file must belong to before anything is written (the file is refused when it names another id), and --dry-run to see the target without writing. This restores the entity's fields and values; it does not undo a move, publish state, or delete.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--id` | string | — | Assert the envelope belongs to this id before writing (refuses otherwise) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco membertype restore-backup <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype restore-backup <file> [flags]
```

### update

```bash
umbraco membertype update <id>
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
umbraco membertype update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco membertype update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco membertype --help

# Inspect a specific endpoint schema
umbraco schema membertype.<method>
```
