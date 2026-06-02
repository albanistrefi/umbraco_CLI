---
name: umbraco-template
description: "Template operations"
metadata:
  version: 0.3.13
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# template

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco template <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `template get <id>` | Get template by ID |
| `template root` | Get root templates |
| `template search` | Search templates |

### get

```bash
umbraco template get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### root

```bash
umbraco template root
```

### search

```bash
umbraco template search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Query parameters as JSON |
| `--query` | string | — | Search query |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `template create` | Create template |
| `template update <id>` | Update template |

### create

```bash
umbraco template create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco template create --dry-run

# 2. Execute
umbraco template create
```

### update

```bash
umbraco template update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco template update <id> --dry-run

# 2. Execute
umbraco template update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco template --help

# Inspect a specific endpoint schema
umbraco schema template.<method>
```
