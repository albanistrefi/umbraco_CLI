---
name: umbraco-searcher
description: "Examine searcher queries"
metadata:
  version: 0.5.5
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

'searcher query' takes the INDEX name (ExternalIndex, InternalIndex, …) that 'indexer list' reports in its name field.
The searcherName 'indexer list' also reports (ExternalSearcher) answers 404 on Umbraco 18.2, and 'searcher list'
returns an empty list there: that is how the Management API behaves, not a sign that search is broken.

Task → command:
  Which names can I query?                             indexer list --fields name
  Why is this page missing from search?                searcher query ExternalIndex --term <text>
  Is the index healthy?                                indexer list
  Rebuild an index                                     indexer rebuild <index-name> --force --wait
  Only show ids and scores                             searcher query ExternalIndex --term <text> --fields id,score
  'searcher list' came back empty                      expected on 18.2; use indexer list --fields name for the names
```

## Read Commands

| Command | Description |
|---------|-------------|
| `searcher list` | List Examine searchers (paginated; --skip/--take/--all) |
| `searcher query <index-name>` | Run a term against one Examine index (paginated; --skip/--take/--all) |

### list

```bash
umbraco searcher list
```

GET /searcher. Lists the searchers the server registers on their own. On Umbraco 18.2 this returns an empty list: that is how the Management API behaves, not a sign that search is broken. 'searcher query' takes the index name, which 'indexer list --fields name' lists (ExternalIndex, InternalIndex).

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
umbraco searcher query <index-name>
```

GET /searcher/{searcherName}/query?term=. Returns the raw Examine hits (id, score, fields) so you can tell "the document is not in the index" apart from "the index has it under different values". Pass the index name from 'indexer list --fields name', e.g. ExternalIndex or InternalIndex: on Umbraco 18.2 the searcherName 'indexer list' reports (ExternalSearcher) answers 404 "Could not find a valid searcher", and 'searcher list' is empty. --query is an alias of --term.

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
