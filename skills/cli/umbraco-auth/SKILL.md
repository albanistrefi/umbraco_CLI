---
name: umbraco-auth
description: "Persistent authentication helpers"
metadata:
  version: 0.4.4
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# auth

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco auth <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `auth list` | List stored auth profiles without exposing secrets |
| `auth status` | Show the current auth/config status without exposing secrets |
| `auth use <profile>` | Set the active stored auth profile |

### list

```bash
umbraco auth list
```

### status

```bash
umbraco auth status
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | bool | false | List command permission requirements for the resolved user context |

### use

```bash
umbraco auth use <profile>
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `auth login` | Store Umbraco API credentials in the user config after verifying them |
| `auth logout` | Remove stored credentials from the user config |

### login

```bash
umbraco auth login
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--base-url` | string | — | Umbraco base URL |
| `--client-id` | string | — | Management API client ID |
| `--client-secret` | string | — | Management API client secret |
| `--dry-run` | bool | false | Verify credentials without persisting them |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco auth login --dry-run

# 2. Execute
umbraco auth login
```

### logout

```bash
umbraco auth logout
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Preview logout without modifying the user config |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco auth logout --dry-run

# 2. Execute
umbraco auth logout
```

## Discovering Commands

```bash
# Browse subcommands
umbraco auth --help

# Inspect a specific endpoint schema
umbraco schema auth.<method>
```
