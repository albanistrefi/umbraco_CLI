---
name: umbraco-doctype
description: "Document type schema operations"
metadata:
  version: 0.2.6
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
| `doctype children <id>` | Get child document types |
| `doctype get <id>` | Get document type by ID |
| `doctype list` | List document types |
| `doctype root` | Get root document types |
| `doctype search` | Search document types |

### children

```bash
umbraco doctype children <id>
```

### get

```bash
umbraco doctype get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### list

```bash
umbraco doctype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### root

```bash
umbraco doctype root
```

### search

```bash
umbraco doctype search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Query parameters as JSON |
| `--query` | string | — | Search query |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `doctype copy <id>` | Copy document type |
| `doctype create` | Create document type |
| `doctype move <id>` | Move document type |
| `doctype update <id>` | Update document type |

### copy

```bash
umbraco doctype copy <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Copy payload as JSON |
| `--to` | string | — | Target parent ID |

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
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create payload as JSON |

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
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Move payload as JSON |
| `--to` | string | — | Target parent ID |

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
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON |

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
