---
name: umbraco-engage
description: "Umbraco Engage operations (analytics, segments, personas, journeys, goals, A/B tests, personalization)"
metadata:
  version: 0.4.27
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
Commands for the Umbraco Engage Management API (/umbraco/engage/management/api/v1). Requires Umbraco Engage on the target instance. What a command may read or change is governed by the permissions of the API user the CLI authenticates as.

Engage entities carry two ids: a numeric `id` and a GUID `unique` (`key` on goals and traffic filters). Most get, update and delete commands take the GUID; the A/B test reads, annotations and segment priorities take the numeric id. Each command says which, and rejects the other kind before calling the API.

Engage saves an entity by POSTing the whole entity to one route for both create and update; 'create' always sends `id` 0, and 'update' fetches the entity first so it can never create one by accident. Every write takes --dry-run; deletes, the main switch and reporting regeneration also require --force.

Start with 'umbraco engage status': it reports the license, the main switch, and whether Engage's data is reachable. When Engage's database migration is incomplete every data route answers HTTP 409 "Umbraco Engage is unavailable" (exit code 4).

Visitor profiles (/profile/*, per-visitor scoring locks, suspicious-visitor routes) are deliberately not exposed: they hold personal data about individual visitors.
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
| `engage main-switch get` | Show whether the main switch is on (GET /main-switch) |
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

Engage 18.1.0 answers HTTP 500 for the visitorType and usertype dimensions; the CLI keeps the error (exit code 4) and adds a hint.

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

GET /annotations/all by default; --global uses /annotations/global (annotations not tied to a page); --node <documentGuid> uses /annotations/page, with --culture for one culture.

--from and --to (YYYY-MM-DD or RFC 3339) are instants, unlike the whole days of 'analytics query': Engage returns the annotations timestamped between them, honouring the time and any offset. A YYYY-MM-DD value is midnight at the start of that day, so --to 2026-09-30 leaves out annotations made on the 30th; pass --to 2026-10-01 to include them. Engage 18.1.0 answers HTTP 500 unless both are given.

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

### main-switch get

```bash
umbraco engage main-switch get
```

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
| `engage annotation create` | Create an analytics annotation (POST /annotations) |
| `engage annotation delete <id>` | Permanently delete an annotation by its numeric `id` |
| `engage campaign-group create` | Create a campaign group (POST /campaign-group) |
| `engage campaign-group delete <unique>` | Permanently delete a campaign group by its GUID `unique` |
| `engage campaign-group update <unique>` | Update a campaign group by its GUID `unique` (POST /campaign-group) |
| `engage goal create` | Create a goal (POST /goal) |
| `engage goal update <key>` | Update a goal by its GUID `key` (POST /goal) |
| `engage journey create` | Create a customer journey (POST /customer-journey) |
| `engage journey delete <unique>` | Permanently delete a customer journey by its GUID `unique` |
| `engage journey update <unique>` | Update a customer journey by its GUID `unique` (POST /customer-journey) |
| `engage main-switch off` | Turn Engage off site-wide (POST /main-switch/turn-off) |
| `engage main-switch on` | Turn Engage on site-wide (POST /main-switch/turn-on) |
| `engage persona create` | Create a persona (POST /persona) |
| `engage persona delete <unique>` | Permanently delete a persona by its GUID `unique` |
| `engage persona update <unique>` | Update a persona by its GUID `unique` (POST /persona) |
| `engage personalization create` | Create a applied personalization (POST /applied-personalization) |
| `engage personalization delete <unique>` | Permanently delete a applied personalization by its GUID `unique` |
| `engage personalization update <unique>` | Update a applied personalization by its GUID `unique` (POST /applied-personalization) |
| `engage referral-group create` | Create a referral group (POST /referral-group) |
| `engage referral-group delete <unique>` | Permanently delete a referral group by its GUID `unique` |
| `engage referral-group update <unique>` | Update a referral group by its GUID `unique` (POST /referral-group) |
| `engage reporting generate` | Regenerate the reporting tables (POST /reporting/generation/start) |
| `engage segment create` | Create a segment (POST /segments) |
| `engage segment delete <unique>` | Permanently delete a segment by its GUID `unique` |
| `engage segment update <unique>` | Update a segment by its GUID `unique` (POST /segments) |
| `engage segment update-priority` | Reorder segment priority (POST /segments/update-priority) |
| `engage traffic-filter create` | Create a traffic filter (POST /traffic-filter) |
| `engage traffic-filter delete <key>` | Permanently delete a traffic filter by its GUID `key` |
| `engage traffic-filter update <key>` | Update a traffic filter by its GUID `key` (POST /traffic-filter) |

### analytics query

```bash
umbraco engage analytics query
```

POST /analytics/query. The request is a POST but reads only.

--metrics is required; --dimensions is optional (none gives one total row). Names are checked against 'analytics metrics' / 'analytics dimensions'. --from and --to are whole days, both inclusive: Engage counts every day from --from through --to and ignores any time of day, so --from 2026-10-01 --to 2026-10-01 is that one day. Pass days as YYYY-MM-DD. An RFC 3339 value is accepted only at midnight UTC (00:00:00Z) and is sent as its date; any other time or offset is refused, because Engage would drop the time and convert an offset to its UTC date. Without them the range is the last 30 days: today (UTC) and the 29 days before.

--filter takes Engage's filter syntax: Dimension=='value' clauses joined with ';' (AND), e.g. "country=='Denmark';deviceCategory=='mobile'". --node <documentGuid> narrows to one page the way the back office does (NodeId=='<guid>', with a '+' suffix under --include-subpages); --culture adds NodeCulture.

--sort defaults to the first dimension. --page is 1-based. The result carries `columns` and `rows` (one array per row, in column order) plus paging totals.

--json sends a full AnalyticsQueryGetModel body verbatim and cannot be combined with the builder flags.

Engage 18.1.0 answers some combinations with HTTP 500 and an empty body; the CLI keeps the error (exit code 4) and adds a hint. Measured on 18.1.0: without --realtime, the year, month, week and day dimensions; the visitorType and usertype dimensions; totalEvents by pagePath; an eventCategory/eventAction filter with pageviews, sessions or goalCompletionsAll; and a goal filter with sessions or users. With --realtime, the totalEvents metric.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ascending` | bool | false | Sort ascending instead of descending |
| `--culture` | string | — | With --node: restrict to one culture, e.g. en-US |
| `--dimensions` | string | — | Comma-separated dimension names, e.g. date or pagePath,country |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--filter` | string | — | Engage filter expression: Dimension=='value' clauses joined with ';' |
| `--from` | string | — | First day of the range, inclusive, as YYYY-MM-DD (default: 29 days before today, UTC) |
| `--include-subpages` | bool | false | With --node: include the page's descendants |
| `--json` | string | — | Full AnalyticsQueryGetModel body, sent verbatim |
| `--metrics` | string | — | Comma-separated metric names (required unless --json), e.g. pageviews,sessions |
| `--node` | string | — | Restrict to one page by its document GUID |
| `--page` | int | 1 | Result page, 1-based |
| `--page-size` | int | 100 | Rows per page |
| `--realtime` | bool | false | Query the realtime (unaggregated) data instead of the reporting tables |
| `--sort` | string | — | Metric or dimension to sort by (default: the first dimension) |
| `--to` | string | — | Last day of the range, inclusive, as YYYY-MM-DD (default: today, UTC) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage analytics query [flags] --dry-run

# 2. Execute with the same flags
umbraco engage analytics query [flags]
```

### annotation create

```bash
umbraco engage annotation create
```

POST /annotations with `id` 0. Needs `timestamp` (RFC 3339), `description` and `visibility` (Always, Node, NodeAndDescendants, Created, Published, AbTestStart, AbTestEnd); anything but Always pins it to `pageVariants` [{"unique":<document GUID>,"culture":""}]. --print-template prints GET /annotations/empty, the server's blank template.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Annotation payload as JSON |
| `--print-template` | bool | false | Print the server's blank annotation; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage annotation create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage annotation create [flags]
```

### annotation delete

```bash
umbraco engage annotation delete <id>
```

DELETE /annotations?id=<id>. Annotations carry only a numeric `id` (from 'umbraco engage annotation list'). Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage annotation delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage annotation delete <id> --force [flags]
```

### campaign-group create

```bash
umbraco engage campaign-group create
```

POST /campaign-group with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints a built-in scaffold mirroring the back office's. `campaigns[]` holds the UTM matches (utmSource, utmMedium, utmCampaign, ...); deleting a group reverts its campaigns to unscored.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage campaign-group create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage campaign-group create [flags]
```

### campaign-group delete

```bash
umbraco engage campaign-group delete <unique>
```

DELETE /campaign-group?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage campaign-group delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage campaign-group delete <unique> --force [flags]
```

### campaign-group update

```bash
umbraco engage campaign-group update <unique>
```

Fetches GET /campaign-group?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new campaign group), then POSTs /campaign-group with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `campaigns[]` holds the UTM matches (utmSource, utmMedium, utmCampaign, ...); deleting a group reverts its campaigns to unscored.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage campaign-group update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage campaign-group update <unique> [flags]
```

### goal create

```bash
umbraco engage goal create
```

POST /goal with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints a built-in scaffold mirroring the back office's. `goalTypeId` comes from 'umbraco engage goal types'. Like the back office, `isActive` defaults to false. Engage answers the goal's GUID. Engage's Management API has no goal delete route; deactivate a goal with --merge-json '{"isActive":false}'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage goal create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage goal create [flags]
```

### goal update

```bash
umbraco engage goal update <key>
```

Fetches GET /goal/details?id=<key> (also under --dry-run, so an unknown GUID fails instead of creating a new goal), then POSTs /goal with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `goalTypeId` comes from 'umbraco engage goal types'. Like the back office, `isActive` defaults to false. Engage answers the goal's GUID. Engage's Management API has no goal delete route; deactivate a goal with --merge-json '{"isActive":false}'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage goal update <key> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage goal update <key> [flags]
```

### journey create

```bash
umbraco engage journey create
```

POST /customer-journey with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints GET /customer-journey/empty, the server's blank template. A journey holds its steps in `steps[]`; Engage answers {journey, validationResults} and a false `isValid` exits 4.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage journey create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage journey create [flags]
```

### journey delete

```bash
umbraco engage journey delete <unique>
```

DELETE /customer-journey?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage journey delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage journey delete <unique> --force [flags]
```

### journey update

```bash
umbraco engage journey update <unique>
```

Fetches GET /customer-journey/details?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new customer journey), then POSTs /customer-journey with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). A journey holds its steps in `steps[]`; Engage answers {journey, validationResults} and a false `isValid` exits 4.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage journey update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage journey update <unique> [flags]
```

### main-switch off

```bash
umbraco engage main-switch off
```

POST /main-switch/turn-off (no body), then reads GET /main-switch back. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the site-wide switch |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage main-switch off [flags] --dry-run

# 2. Execute with the same flags
umbraco engage main-switch off --force [flags]
```

### main-switch on

```bash
umbraco engage main-switch on
```

POST /main-switch/turn-on (no body), then reads GET /main-switch back. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the site-wide switch |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage main-switch on [flags] --dry-run

# 2. Execute with the same flags
umbraco engage main-switch on --force [flags]
```

### persona create

```bash
umbraco engage persona create
```

POST /persona with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints GET /persona/empty, the server's blank template. A persona group holds its personas in `personas[]`; Engage answers {persona, validationResults} and a false `isValid` exits 4.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage persona create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage persona create [flags]
```

### persona delete

```bash
umbraco engage persona delete <unique>
```

DELETE /persona?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage persona delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage persona delete <unique> --force [flags]
```

### persona update

```bash
umbraco engage persona update <unique>
```

Fetches GET /persona/details?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new persona), then POSTs /persona with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). A persona group holds its personas in `personas[]`; Engage answers {persona, validationResults} and a false `isValid` exits 4.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage persona update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage persona update <unique> [flags]
```

### personalization create

```bash
umbraco engage personalization create
```

POST /applied-personalization with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints a built-in scaffold mirroring the back office's. `segmentId` is the segment's numeric `id`. Like the back office, a `ContentType` personalization is saved with `pages` emptied and any other type with `contentTypes` emptied.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage personalization create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage personalization create [flags]
```

### personalization delete

```bash
umbraco engage personalization delete <unique>
```

DELETE /applied-personalization?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage personalization delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage personalization delete <unique> --force [flags]
```

### personalization update

```bash
umbraco engage personalization update <unique>
```

Fetches GET /applied-personalization/id?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new applied personalization), then POSTs /applied-personalization with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `segmentId` is the segment's numeric `id`. Like the back office, a `ContentType` personalization is saved with `pages` emptied and any other type with `contentTypes` emptied.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage personalization update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage personalization update <unique> [flags]
```

### referral-group create

```bash
umbraco engage referral-group create
```

POST /referral-group with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints a built-in scaffold mirroring the back office's. `pages[]` holds the referring pages (pageUrl, domainOnly).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage referral-group create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage referral-group create [flags]
```

### referral-group delete

```bash
umbraco engage referral-group delete <unique>
```

DELETE /referral-group?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage referral-group delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage referral-group delete <unique> --force [flags]
```

### referral-group update

```bash
umbraco engage referral-group update <unique>
```

Fetches GET /referral-group?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new referral group), then POSTs /referral-group with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `pages[]` holds the referring pages (pageUrl, domainOnly).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage referral-group update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage referral-group update <unique> [flags]
```

### reporting generate

```bash
umbraco engage reporting generate
```

POST /reporting/generation/start (no body) starts a regeneration of Engage's aggregated reporting tables in the background; follow it with 'umbraco engage reporting status'. The back office warns that regenerating can affect site performance, so this requires --force (or --dry-run to rehearse). A 409 other than "Umbraco Engage is unavailable" means a generation is already running.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the regeneration |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage reporting generate [flags] --dry-run

# 2. Execute with the same flags
umbraco engage reporting generate --force [flags]
```

### segment create

```bash
umbraco engage segment create
```

POST /segments with `id` 0. --json is merged onto the create defaults; `unique` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints a built-in scaffold mirroring the back office's. `controlGroupSize` is a fraction (0.2 = 20%), not the percentage the back office displays; rules come from 'umbraco engage segment get' on an existing segment.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage segment create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage segment create [flags]
```

### segment delete

```bash
umbraco engage segment delete <unique>
```

DELETE /segments?id=<unique>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage segment delete <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage segment delete <unique> --force [flags]
```

### segment update

```bash
umbraco engage segment update <unique>
```

Fetches GET /segments?id=<unique> (also under --dry-run, so an unknown GUID fails instead of creating a new segment), then POSTs /segments with the entity's numeric `id` and `unique` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `controlGroupSize` is a fraction (0.2 = 20%), not the percentage the back office displays; rules come from 'umbraco engage segment get' on an existing segment.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage segment update <unique> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage segment update <unique> [flags]
```

### segment update-priority

```bash
umbraco engage segment update-priority
```

POST /segments/update-priority with a bare array of {id, sortOrder}. A visitor in several segments gets the content of the highest-priority one. --order 7,3,9 lists numeric segment `id`s (from 'umbraco engage segment list') highest priority first and sends sortOrder 0, 1, 2, ...; --json sends an array verbatim.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Raw [{"id":7,"sortOrder":0},...] array |
| `--order` | string | — | Comma-separated numeric segment ids, highest priority first |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage segment update-priority [flags] --dry-run

# 2. Execute with the same flags
umbraco engage segment update-priority [flags]
```

### traffic-filter create

```bash
umbraco engage traffic-filter create
```

POST /traffic-filter with `id` 0. --json is merged onto the create defaults; `key` is generated when omitted. A non-zero `id` is rejected: Engage would update that entity instead, so use 'update'. --print-template prints GET /traffic-filter/empty, the server's blank template. `mode` is Block, Filter or BlockAndFilter. Engage answers the saved filter's `key`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON (merged onto the create defaults) |
| `--print-template` | bool | false | Print a JSON skeleton; edit it and pass it to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage traffic-filter create [flags] --dry-run

# 2. Execute with the same flags
umbraco engage traffic-filter create [flags]
```

### traffic-filter delete

```bash
umbraco engage traffic-filter delete <key>
```

DELETE /traffic-filter?key=<key>. Requires --force (or --dry-run to rehearse).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage traffic-filter delete <key> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage traffic-filter delete <key> --force [flags]
```

### traffic-filter update

```bash
umbraco engage traffic-filter update <key>
```

Fetches GET /traffic-filter?key=<key> (also under --dry-run, so an unknown GUID fails instead of creating a new traffic filter), then POSTs /traffic-filter with the entity's numeric `id` and `key` pinned. Pass exactly one of --json (full replacement) or --merge-json (deep-merged into the fetched entity; arrays such as rules or scoring entries are replaced wholesale, not merged per entry). `mode` is Block, Filter or BlockAndFilter. Engage answers the saved filter's `key`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current entity before the save (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco engage traffic-filter update <key> [flags] --dry-run

# 2. Execute with the same flags
umbraco engage traffic-filter update <key> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco engage --help

# Inspect a specific endpoint schema
umbraco schema engage.<method>
```
