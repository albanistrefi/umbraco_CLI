---
name: umbraco-doctype
description: "Document type schema operations"
metadata:
  version: 0.4.0
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

## Read Commands

| Command | Description |
|---------|-------------|
| `doctype children <id>` | Get child document types (paginated; --skip/--take/--all) |
| `doctype get <id>` | Get document type by ID |
| `doctype list` | List document types (paginated; --skip/--take/--all) |
| `doctype root` | Get root document types (paginated; --skip/--take/--all) |
| `doctype search` | Search document types |

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
umbraco doctype get <id>
```

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
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

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
| `doctype add-property <id>` | Append a property to a document type under an existing container alias |
| `doctype copy <id>` | Copy document type |
| `doctype create` | Create document type (pass --element to create an element type) |
| `doctype move <id>` | Move document type |
| `doctype update <id>` | Update document type |

### add-container

```bash
umbraco doctype add-container <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Display name for the new container |
| `--parent` | string | — | Optional name of an existing parent container (typically a Tab when adding a Group) |
| `--type` | string | — | Container type: Tab or Group |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco doctype add-container <id> --dry-run

# 2. Execute
umbraco doctype add-container <id>
```

### add-property

```bash
umbraco doctype add-property <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alias` | string | — | Property alias (camelCase identifier) |
| `--container` | string | — | Name of the existing tab/group container that should hold the property (case-insensitive match) |
| `--data-type` | string | — | Data type ID (GUID) backing the property |
| `--description` | string | — | Optional property description |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--mandatory` | bool | false | Mark the property as mandatory |
| `--name` | string | — | Human-readable property name |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco doctype add-property <id> --dry-run

# 2. Execute
umbraco doctype add-property <id>
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
# 1. Dry run first
umbraco doctype copy <id> --dry-run

# 2. Execute
umbraco doctype copy <id>
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
# 1. Dry run first
umbraco doctype create --dry-run

# 2. Execute
umbraco doctype create
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
# 1. Dry run first
umbraco doctype move <id> --dry-run

# 2. Execute
umbraco doctype move <id>
```

### update

```bash
umbraco doctype update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco doctype update <id> --dry-run

# 2. Execute
umbraco doctype update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco doctype --help

# Inspect a specific endpoint schema
umbraco schema doctype.<method>
```
