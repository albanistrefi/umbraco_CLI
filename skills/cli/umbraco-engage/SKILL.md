---
name: umbraco-engage
description: "Umbraco Engage operations (read-only: analytics, segments, personas, journeys, goals, A/B tests)"
metadata:
  version: 0.4.24
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# engage

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco engage <command> [flags]
```

## Overview

```text
Read-only commands for the Umbraco Engage Management API (/umbraco/engage/management/api/v1). Requires Umbraco Engage on the target instance.

Engage entities carry two ids: a numeric `id` and a GUID `unique` (`key` on goals and traffic filters). Most get commands take the GUID; the A/B test reads take the numeric id. Each get command says which, and rejects the other kind before calling the API.

Start with 'umbraco engage status': it reports the license, the main switch, and whether Engage's data is reachable. When Engage's database migration is incomplete every data read answers HTTP 409 "Umbraco Engage is unavailable" (exit code 4).

Visitor profiles (/profile/*) are deliberately not exposed: they return personal data about individual visitors.
```

## Read Commands

| Command | Description |
|---------|-------------|
| `engage abtest get <id>` | Get one A/B test by its numeric `id` |
| `engage abtest list` | List every A/B test (GET /ab-test/all) |
| `engage abtest project <unique>` | Get one A/B test project with its tests by the project's GUID `unique` |
| `engage abtest projects` | List every A/B test project (GET /ab-test-project/all) |
| `engage abtest variants <abTestId>` | List the variants of one A/B test by the test's numeric `id` |
| `engage analytics dimensions` | List the dimension names 'analytics query --dimensions' and 'analytics distinct' accept |
| `engage analytics distinct` | List the distinct values recorded for one dimension (GET /analytics/distinct) |
| `engage analytics metrics` | List the metric names 'analytics query --metrics' accepts |
| `engage annotation list` | List annotations, optionally only global ones or those on one page |
| `engage campaign-group get <unique>` | Get one campaign group by its GUID `unique` |
| `engage campaign-group list` | List every campaign group (GET /campaign-group/all) |
| `engage config` | Show Engage's effective configuration (analytics, A/B testing, segmentation, reporting settings) |
| `engage goal get <key>` | Get one goal with its full configuration by its GUID `key` |
| `engage goal list` | List every goal (GET /goals/all) |
| `engage goal main` | List the main (macro) goals (GET /goals/main) |
| `engage goal types` | List the goal types goals can be configured with (GET /goal/all/types) |
| `engage journey get <unique>` | Get one customer journey by its GUID `unique` |
| `engage journey list` | List every customer journey (GET /customer-journey/all) |
| `engage persona get <unique>` | Get one persona by its GUID `unique` |
| `engage persona list` | List every persona (GET /persona/all) |
| `engage personalization get <unique>` | Get one applied personalization by its GUID `unique` |
| `engage personalization list` | List every applied personalization (GET /applied-personalization/all) |
| `engage referral-group get <unique>` | Get one referral group by its GUID `unique` |
| `engage referral-group list` | List every referral group (GET /referral-group/all) |
| `engage reporting status` | Whether the reporting tables exist, are being generated, and when they were last generated (GET /reporting/generation/status) |
| `engage segment get <unique>` | Get one segment by its GUID `unique` |
| `engage segment list` | List every segment (GET /segments/all) |
| `engage stats identification` | Newly identified vs. still-unknown profiles over a window (GET /profile/statistics/identification) |
| `engage stats overview` | Totals of pageviews, person and bot visitors, events, and segment settings (GET /statistics) |
| `engage stats profile-growth` | Monthly counts of identified and unknown profiles (GET /profile/statistics/growth) |
| `engage stats profiles` | Count identified and unknown profiles (GET /profile/statistics/total) |
| `engage status` | Report Engage's version, license, main switch, add-ons, and whether its data is reachable |
| `engage traffic-filter get <key>` | Get one traffic filter by its GUID `key` |
| `engage traffic-filter list` | List every traffic filter (GET /traffic-filter/all) |

### abtest get

```bash
umbraco engage abtest get <id>
```

GET /ab-test?id=<id>. Takes the numeric `id` from 'umbraco engage abtest list', not the GUID `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### abtest list

```bash
umbraco engage abtest list
```

GET /ab-test/all. Returns a bare array, not paged. 'abtest get' and 'abtest variants' take an entry's numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### abtest project

```bash
umbraco engage abtest project <unique>
```

GET /ab-test-project/details?id=<unique>. Takes the GUID `unique` from 'umbraco engage abtest projects', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### abtest projects

```bash
umbraco engage abtest projects
```

GET /ab-test-project/all. A project groups the A/B tests run on one page; 'abtest project' takes an entry's GUID `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### abtest variants

```bash
umbraco engage abtest variants <abTestId>
```

GET /ab-test-variant/all?abTestId=<id>. Takes the numeric `id` from 'umbraco engage abtest list'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### analytics dimensions

```bash
umbraco engage analytics dimensions
```

List the dimension names 'analytics query --dimensions' and 'analytics distinct' accept. Taken from the Engage 18.1.0 OpenAPI document; names are matched case-insensitively.

### analytics distinct

```bash
umbraco engage analytics distinct
```

GET /analytics/distinct?dimension=<name>. Useful for building --filter values, e.g. the countries or device categories Engage has seen.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dimension` | string | — | Dimension name (see 'analytics dimensions') |

### analytics metrics

```bash
umbraco engage analytics metrics
```

List the metric names 'analytics query --metrics' accepts. Taken from the Engage 18.1.0 OpenAPI document; names are matched case-insensitively.

### annotation list

```bash
umbraco engage annotation list
```

GET /annotations/all by default; --global uses /annotations/global (annotations not tied to a page); --node <documentGuid> uses /annotations/page, with --culture for one culture. --from/--to (YYYY-MM-DD or RFC 3339) bound the range.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | With --node: only this culture |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--from` | string | — | Start of the range: YYYY-MM-DD or RFC 3339 |
| `--global` | bool | false | Only annotations not tied to a page |
| `--node` | string | — | Only annotations on this page (document GUID) |
| `--to` | string | — | End of the range: YYYY-MM-DD or RFC 3339 |

### campaign-group get

```bash
umbraco engage campaign-group get <unique>
```

GET /campaign-group?id=<unique>. Pass the GUID `unique` from 'umbraco engage campaign-group list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### campaign-group list

```bash
umbraco engage campaign-group list
```

GET /campaign-group/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### config

```bash
umbraco engage config
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### goal get

```bash
umbraco engage goal get <key>
```

GET /goal/details?id=<key>. Pass the GUID `key` from 'umbraco engage goal list' (the detail model calls it `unique`), not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### goal list

```bash
umbraco engage goal list
```

GET /goals/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `key`; 'goal get' takes the `key`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### goal main

```bash
umbraco engage goal main
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### goal types

```bash
umbraco engage goal types
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### journey get

```bash
umbraco engage journey get <unique>
```

GET /customer-journey/details?id=<unique>. Pass the GUID `unique` from 'umbraco engage journey list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### journey list

```bash
umbraco engage journey list
```

GET /customer-journey/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### persona get

```bash
umbraco engage persona get <unique>
```

GET /persona/details?id=<unique>. Pass the GUID `unique` from 'umbraco engage persona list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### persona list

```bash
umbraco engage persona list
```

GET /persona/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### personalization get

```bash
umbraco engage personalization get <unique>
```

GET /applied-personalization/id?id=<unique>. Pass the GUID `unique` from 'umbraco engage personalization list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### personalization list

```bash
umbraco engage personalization list
```

GET /applied-personalization/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### referral-group get

```bash
umbraco engage referral-group get <unique>
```

GET /referral-group?id=<unique>. Pass the GUID `unique` from 'umbraco engage referral-group list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### referral-group list

```bash
umbraco engage referral-group list
```

GET /referral-group/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### reporting status

```bash
umbraco engage reporting status
```

GET /reporting/generation/status. Non-realtime analytics queries read the reporting tables, so a missing or stale generation explains empty or old 'analytics query' results.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### segment get

```bash
umbraco engage segment get <unique>
```

GET /segments?id=<unique>. Pass the GUID `unique` from 'umbraco engage segment list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### segment list

```bash
umbraco engage segment list
```

GET /segments/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `unique`; 'get' takes the `unique`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--days` | int | 0 | Window in days for the per-segment visitor statistics |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--temporary` | bool | false | List temporary (unsaved) segments instead of saved ones |

### stats identification

```bash
umbraco engage stats identification
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--days` | int | 0 | Window in days (server default when unset) |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### stats overview

```bash
umbraco engage stats overview
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### stats profile-growth

```bash
umbraco engage stats profile-growth
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--months` | int | 0 | Number of months to return (server default when unset) |

### stats profiles

```bash
umbraco engage stats profiles
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### status

```bash
umbraco engage status
```

Combines GET /package, /main-switch and /add-ons with a probe of a data route (/goal/all/types). `dataAvailable` is false when Engage answers 409 "Umbraco Engage is unavailable", which it does on every data route while its database schema alignment is incomplete; `unavailable` then carries the server's title and detail. Any other failure (auth, a missing route because Engage is not installed) is returned as an error with its exit code.

### traffic-filter get

```bash
umbraco engage traffic-filter get <key>
```

GET /traffic-filter?key=<key>. Pass the GUID `key` from 'umbraco engage traffic-filter list', not the numeric `id`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### traffic-filter list

```bash
umbraco engage traffic-filter list
```

GET /traffic-filter/all. Returns a bare array, not paged. Each entry carries a numeric `id` and the GUID `key`; 'get' takes the `key`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `engage analytics query` | Run an analytics query (POST /analytics/query; read-only) |

### analytics query

```bash
umbraco engage analytics query
```

POST /analytics/query. The request is a POST but reads only.

--metrics is required; --dimensions is optional (none gives one total row). Names are checked against 'analytics metrics' / 'analytics dimensions'. --from/--to default to the last 30 days ending now (UTC); YYYY-MM-DD values are sent as given, which the server reads as midnight, so to include a whole --to day pass the next day or a full RFC 3339 timestamp.

--filter takes Engage's filter syntax: Dimension=='value' clauses joined with ';' (AND), e.g. "country=='Denmark';deviceCategory=='mobile'". --node <documentGuid> narrows to one page the way the back office does (NodeId=='<guid>', with a '+' suffix under --include-subpages); --culture adds NodeCulture.

--sort defaults to the first dimension. --page is 1-based. The result carries `columns` and `rows` (one array per row, in column order) plus paging totals.

--json sends a full AnalyticsQueryGetModel body verbatim and cannot be combined with the builder flags.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ascending` | bool | false | Sort ascending instead of descending |
| `--culture` | string | — | With --node: restrict to one culture, e.g. en-US |
| `--dimensions` | string | — | Comma-separated dimension names, e.g. date or pagePath,country |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--filter` | string | — | Engage filter expression: Dimension=='value' clauses joined with ';' |
| `--from` | string | — | Start of the range: YYYY-MM-DD or RFC 3339 (default: 30 days ago, midnight UTC) |
| `--include-subpages` | bool | false | With --node: include the page's descendants |
| `--json` | string | — | Full AnalyticsQueryGetModel body, sent verbatim |
| `--metrics` | string | — | Comma-separated metric names (required unless --json), e.g. pageviews,sessions |
| `--node` | string | — | Restrict to one page by its document GUID |
| `--page` | int | 1 | Result page, 1-based |
| `--page-size` | int | 100 | Rows per page |
| `--realtime` | bool | false | Query the realtime (unaggregated) data instead of the reporting tables |
| `--sort` | string | — | Metric or dimension to sort by (default: the first dimension) |
| `--to` | string | — | End of the range: YYYY-MM-DD or RFC 3339 (default: now) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage analytics query [flags] --dry-run

# 2. Execute with the same flags
umbraco engage analytics query [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco engage --help

# Inspect a specific endpoint schema
umbraco schema engage.<method>
```
