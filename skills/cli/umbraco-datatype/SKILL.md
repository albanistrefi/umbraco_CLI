---
name: umbraco-datatype
description: "Data type operations"
metadata:
  version: 0.3.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# datatype

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco datatype <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `datatype block` | Manage allowed blocks on a Block List / Block Grid datatype |
| `datatype extensions <id>` | List enabled data type extension aliases |
| `datatype get <id>` | Get data type by ID |
| `datatype is-used <id>` | Check whether a data type is in use |
| `datatype list` | List data types |
| `datatype root` | Get root data types |
| `datatype search` | Search data types |

### block

```bash
umbraco datatype block
```

Read-modify-write helpers that mutate the 'blocks' value entry on Umbraco.BlockList and Umbraco.BlockGrid datatypes without clobbering the rest of the configuration. Idempotent: 'add' is a no-op if the element type is already an allowed block; 'remove' is a no-op if it isn't.

### extensions

```bash
umbraco datatype extensions <id>
```

### get

```bash
umbraco datatype get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### is-used

```bash
umbraco datatype is-used <id>
```

### list

```bash
umbraco datatype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | 0 | Pagination offset |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 100 | Pagination page size |

### root

```bash
umbraco datatype root
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | 0 | Pagination offset |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 100 | Pagination page size |

### search

```bash
umbraco datatype search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--editor-alias` | string | — | Filter by property editor alias, e.g. Umbraco.TextBox |
| `--params` | string | — | Query parameters as JSON |
| `--query` | string | — | Search query |
| `--skip` | int | 0 | Pagination offset |
| `--take` | int | 100 | Pagination page size |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `datatype add-extension <id> <extension-alias>` | Add an extension alias to the datatype extensions array |
| `datatype add-value <id>` | Append a string value to a datatype array setting |
| `datatype create` | Create data type |
| `datatype remove-extension <id> <extension-alias>` | Remove an extension alias from the datatype extensions array |
| `datatype remove-value <id>` | Remove a string value from a datatype array setting |
| `datatype update <id>` | Update data type |

### add-extension

```bash
umbraco datatype add-extension <id> <extension-alias>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype add-extension <id> <extension-alias> --dry-run

# 2. Execute
umbraco datatype add-extension <id> <extension-alias>
```

### add-value

```bash
umbraco datatype add-value <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Datatype array alias to update |
| `--dry-run` | bool | false | Validate request without executing |
| `--value` | string | — | String value to append |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype add-value <id> --dry-run

# 2. Execute
umbraco datatype add-value <id>
```

### create

```bash
umbraco datatype create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype create --dry-run

# 2. Execute
umbraco datatype create
```

### remove-extension

```bash
umbraco datatype remove-extension <id> <extension-alias>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype remove-extension <id> <extension-alias> --dry-run

# 2. Execute
umbraco datatype remove-extension <id> <extension-alias>
```

### remove-value

```bash
umbraco datatype remove-value <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Datatype array alias to update |
| `--dry-run` | bool | false | Validate request without executing |
| `--value` | string | — | String value to remove |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype remove-value <id> --dry-run

# 2. Execute
umbraco datatype remove-value <id>
```

### update

```bash
umbraco datatype update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON; merged into the current data type so fields not mentioned (e.g. editorUiAlias) are preserved |
| `--merge-json` | string | — | Partial JSON payload merged into the current data type before update (alias for --json) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype update <id> --dry-run

# 2. Execute
umbraco datatype update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco datatype --help

# Inspect a specific endpoint schema
umbraco schema datatype.<method>
```
