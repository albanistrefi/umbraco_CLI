---
name: umbraco-health
description: "Health check operations"
metadata:
  version: 0.3.8
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# health

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco health <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `health group <name>` | Get health check group details |
| `health groups` | List health check groups |
| `health run <group-name>` | Run health checks for group |

### group

```bash
umbraco health group <name>
```

### groups

```bash
umbraco health groups
```

### run

```bash
umbraco health run <group-name>
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `health action <action-id>` | Execute a health check action |

### action

```bash
umbraco health action <action-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Action payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco health action <action-id> --dry-run

# 2. Execute
umbraco health action <action-id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco health --help

# Inspect a specific endpoint schema
umbraco schema health.<method>
```
