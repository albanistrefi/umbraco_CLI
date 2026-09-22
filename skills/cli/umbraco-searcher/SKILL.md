---
name: umbraco-searcher
description: "Examine searcher queries"
metadata:
  version: 0.4.22
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# searcher

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco searcher <command> [flags]
```

## Overview

```text
Examine searcher queries.

Task → command:
  Which searchers exist on this instance?              searcher list
  Why is this page missing from search?                searcher query ExternalSearcher --term <text>
  Is the index behind the searcher healthy?            indexer list
  Rebuild the index behind a searcher                  indexer rebuild <index-name> --force --wait
  Only show ids and scores                             searcher query ExternalSearcher --term <text> --fields id,score
  'searcher list' came back empty                      indexer list --fields name,searcherName (try both names)
```

## Read Commands

| Command | Description |
|---------|-------------|
| `searcher list` | List Examine searchers (paginated; --skip/--take/--all) |
| `searcher query <searcher-name>` | Run a term against one Examine searcher (paginated; --skip/--take/--all) |

### list

```bash
umbraco searcher list
```

GET /searcher. Names listed here are the <searcher-name> argument of 'searcher query'. The list only covers searchers registered standalone, so it can be empty on instances that register indexes only: fall back to 'indexer list --fields name,searcherName' and try both names.

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

### query

```bash
umbraco searcher query <searcher-name>
```

GET /searcher/{searcherName}/query?term=. Returns the raw Examine hits (id, score, fields) so you can tell "the document is not in the index" apart from "the index has it under different values". Searcher names come from 'searcher list' or from 'indexer list --fields name,searcherName' — the accepted name is often the index name (ExternalIndex) rather than the searcherName the index reports, so try both when the server answers 404 "Could not find a valid searcher". --query is an alias of --term.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--query` | string | — | Alias of --term, for consistency with the other search commands |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--term` | string | — | Search text sent as the term query parameter (required) |

## Discovering Commands

```bash
# Browse subcommands
umbraco searcher --help

# Inspect a specific endpoint schema
umbraco schema searcher.<method>
```
