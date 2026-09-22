---
name: umbraco-tag
description: "Tag reads across tagged content"
metadata:
  version: 0.4.23
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# tag

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco tag <command> [flags]
```

## Overview

```text
Tag reads across tagged content.

Task → command:
  Which tags exist at all?                 tag list --all
  Which tags start with a prefix?          tag list --query <text>
  Which tags belong to one tag picker?     tag list --group <tagGroup>
  Which tags exist for one culture?        tag list --culture <isoCode>
```

## Read Commands

| Command | Description |
|---------|-------------|
| `tag list` | List tags (paginated; --skip/--take/--all) |

### list

```bash
umbraco tag list
```

GET /tag. --query matches tag text server-side, --group narrows to one tag group (the tagGroup configured on the tag picker data type), --culture narrows to the tags stored for one language. --params wins on key collisions.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--culture` | string | — | Filter by culture ISO code (maps to ?culture=) |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--group` | string | — | Filter by tag group (maps to ?tagGroup=) |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--query` | string | — | Filter tags by text (maps to ?query=) |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Discovering Commands

```bash
# Browse subcommands
umbraco tag --help

# Inspect a specific endpoint schema
umbraco schema tag.<method>
```
