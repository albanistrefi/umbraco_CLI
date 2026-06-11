---
name: umbraco-datatype
description: "Data type operations"
metadata:
  version: 0.4.2
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
| `datatype block list <datatypeId>` | List allowed blocks on a Block List / Block Grid datatype |
| `datatype extensions <id>` | List enabled data type extension aliases |
| `datatype get <id>` | Get data type by ID |
| `datatype is-used <id>` | Check whether a data type is in use |
| `datatype list` | List data types (paginated; --skip/--take/--all) |
| `datatype root` | Get root data types (paginated; --skip/--take/--all) |
| `datatype search` | Search data types |

### block list

```bash
umbraco datatype block list <datatypeId>
```

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
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

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
umbraco datatype root
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
| `datatype block add <datatypeId>` | Register an element type as an allowed block |
| `datatype block remove <datatypeId>` | Unregister an element type from a Block List / Block Grid |
| `datatype block update <datatypeId>` | Update an existing block's properties (partial; flags only mutate what you pass) |
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
| `--dry-run` | bool | false | Print the planned request without executing |

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
| `--dry-run` | bool | false | Print the planned request without executing |
| `--value` | string | — | String value to append |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype add-value <id> --dry-run

# 2. Execute
umbraco datatype add-value <id>
```

### block add

```bash
umbraco datatype block add <datatypeId>
```

Appends a block to the datatype's blocks array. Idempotent: if a block with the same --content-element-type is already present, no PUT is sent.

BlockGrid: --allow-at-root and --allow-in-areas default to true so the block is actually placeable after registration (server-side both default to false when omitted, which would register a block that's invisible to editors). Pass --allow-at-root=false or --allow-in-areas=false to override. --group support over BlockGrid's blockGroups array is a deferred follow-up.

BlockList: --allow-at-root and --allow-in-areas are ignored (those flags only apply to Block Grid).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--allow-at-root` | bool | true | BlockGrid only: allow placing the block at the grid's root level (default true). Ignored for BlockList. |
| `--allow-in-areas` | bool | true | BlockGrid only: allow placing the block inside areas of other blocks (default true). Ignored for BlockList. |
| `--content-element-type` | string | — | GUID of the element type to register as a block (required) |
| `--dry-run` | bool | false | Validate the resulting payload without writing it |
| `--editor-size` | string | — | Overlay size: small | medium | large |
| `--force-hide-content-editor` | bool | false | Hide the content editor in the overlay (settings-only blocks) |
| `--label` | string | — | Optional label shown in the block picker; defaults to the element type's name |
| `--settings-element-type` | string | — | GUID of the element type to use for the block's settings overlay (optional) |
| `--thumbnail` | string | — | Optional path/URL to a thumbnail image |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype block add <datatypeId> --dry-run

# 2. Execute
umbraco datatype block add <datatypeId>
```

### block remove

```bash
umbraco datatype block remove <datatypeId>
```

Idempotent: if no block with --content-element-type is registered, no PUT is sent.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content-element-type` | string | — | GUID of the element type to unregister (required) |
| `--dry-run` | bool | false | Validate the resulting payload without writing it |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype block remove <datatypeId> --dry-run

# 2. Execute
umbraco datatype block remove <datatypeId>
```

### block update

```bash
umbraco datatype block update <datatypeId>
```

Mutates a single existing block on a Block List / Block Grid datatype. The deliberate difference from 'block add': if no block with --content-element-type is present, this errors instead of creating one.

Partial-update semantics: only flags you pass on the command line are applied. Unpassed flags leave that property untouched, so 'block update <dt> --content-element-type <guid> --editor-size large' will not wipe the label.

Clearing optional fields: pass an empty string. --thumbnail "" and --settings-element-type "" remove those fields entirely. --label "" is also accepted and removes the override label (the editor falls back to the element type's name).

Idempotent: if the resulting block is byte-identical to the current one, no PUT is sent.

BlockGrid: --allow-at-root and --allow-in-areas are honored when explicitly passed. Both are ignored for BlockList (mirror of 'block add').

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--allow-at-root` | bool | true | BlockGrid only: allow placing the block at the grid's root level. Ignored for BlockList. |
| `--allow-in-areas` | bool | true | BlockGrid only: allow placing the block inside areas of other blocks. Ignored for BlockList. |
| `--content-element-type` | string | — | GUID of the block to update (required; identity key — same as 'block add' / 'block remove') |
| `--dry-run` | bool | false | Validate the resulting payload without writing it |
| `--editor-size` | string | — | Overlay size: small | medium | large. Pass empty string to clear. |
| `--force-hide-content-editor` | bool | false | Hide the content editor in the overlay (settings-only blocks) |
| `--label` | string | — | New block label. Pass empty string to clear (editor falls back to element type name). |
| `--settings-element-type` | string | — | Set the settings overlay element type. Pass empty string to clear. |
| `--thumbnail` | string | — | Path/URL to a thumbnail image. Pass empty string to clear. |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco datatype block update <datatypeId> --dry-run

# 2. Execute
umbraco datatype block update <datatypeId>
```

### create

```bash
umbraco datatype create
```

POST /data-type. Editor settings go in the values array ([{alias, value}, ...]); a configuration map ({alias: value}) is accepted as a convenience and converted to values automatically.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
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
| `--dry-run` | bool | false | Print the planned request without executing |

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
| `--dry-run` | bool | false | Print the planned request without executing |
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

Updates a data type with the uniform CLI update contract:

  --json        full replacement; the server resets any field not mentioned
                (including editorUiAlias, items, multiple)
  --merge-json  fetches the current data type, deep-merges the patch, and
                PUTs the result; fields not mentioned are preserved

Before v0.4.0 --json silently behaved like --merge-json on this resource.
Pass --merge-json for partial edits. A configuration map ({alias: value})
is accepted as a convenience and converted to the values array the API
expects.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

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
