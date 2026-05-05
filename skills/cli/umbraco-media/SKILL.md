---
name: umbraco-media
description: "Media asset operations"
metadata:
  version: 0.2.7
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# media

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco media <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `media children <id>` | Get child media items |
| `media get <id>` | Get media by ID |
| `media root` | Get root media items |
| `media search` | Search media items |
| `media urls <id>` | Get media URLs |

### children

```bash
umbraco media children <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### get

```bash
umbraco media get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### root

```bash
umbraco media root
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### search

```bash
umbraco media search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |

### urls

```bash
umbraco media urls <id>
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `media create` | Create media from JSON payload |
| `media create-folder [name]` | Create media folder |
| `media move <id>` | Move media item |
| `media trash <id>` | Move media item to recycle bin |
| `media update <id>` | Update media item |

### create

```bash
umbraco media create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco media create --dry-run

# 2. Execute
umbraco media create
```

### create-folder

```bash
umbraco media create-folder [name]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create-folder payload as JSON |
| `--parent` | string | — | Target parent ID |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco media create-folder [name] --dry-run

# 2. Execute
umbraco media create-folder [name]
```

### move

```bash
umbraco media move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Move payload as JSON |
| `--to` | string | — | Target parent ID |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco media move <id> --dry-run

# 2. Execute
umbraco media move <id>
```

### trash

```bash
umbraco media trash <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco media trash <id> --dry-run

# 2. Execute
umbraco media trash <id>
```

### update

```bash
umbraco media update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco media update <id> --dry-run

# 2. Execute
umbraco media update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco media --help

# Inspect a specific endpoint schema
umbraco schema media.<method>
```
