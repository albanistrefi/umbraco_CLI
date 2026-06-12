---
name: umbraco-member-group
description: "Member group lookups (for 'member set-groups' GUID discovery)"
metadata:
  version: 0.4.3
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# member-group

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco member-group <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `member-group get <id>` | Get a member group by ID |
| `member-group list` | List all member groups |

### get

```bash
umbraco member-group get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### list

```bash
umbraco member-group list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |

## Discovering Commands

```bash
# Browse subcommands
umbraco member-group --help

# Inspect a specific endpoint schema
umbraco schema member-group.<method>
```
