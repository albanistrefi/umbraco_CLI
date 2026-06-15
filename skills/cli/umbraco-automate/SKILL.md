---
name: umbraco-automate
description: "Umbraco Automate operations (event-driven workflow automation)"
metadata:
  version: 0.4.4
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# automate

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco automate <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `automate approvals pending` | List approvals waiting for a decision |
| `automate automation ancestors <id>` | Get an automation's location (workspace and group breadcrumb) |
| `automate automation export <id>` | Export an automation as a portable definition |
| `automate automation get <id>` | Get an automation by ID (trigger, steps, connections, state) |
| `automate automation list` | List automations (paginated; --skip/--take/--all) |
| `automate automation runs <id>` | List runs for an automation (paginated; --skip/--take/--all) |
| `automate automation validate --workspace-id <id> --file <export.json>` | Validate an automation definition server-side without writing anything |
| `automate catalogue actions` | List action step types |
| `automate catalogue connection-types` | List connection types |
| `automate catalogue control-flows` | List control-flow step types |
| `automate catalogue notification-channels` | List notification channels |
| `automate catalogue output-schema <alias>` | Resolve a step type's dynamic output schema |
| `automate catalogue step-types` | List step types, optionally filtered by kind |
| `automate catalogue triggers` | List trigger step types |
| `automate catalogue webhook-authenticators` | List webhook authenticators |
| `automate connection get <id>` | Get a connection by ID |
| `automate connection list` | List connections (paginated; --skip/--take/--all) |
| `automate metrics by-automation` | Get run metrics grouped by automation |
| `automate metrics summary` | Get run summary metrics |
| `automate run get <id>` | Get a run by ID (per-step status, errors, retries, timing -- resolved step values are not exposed by the API) |
| `automate version-history compare <entity-type> <entity-id> <from-version> <to-version>` | Compare two stored versions of an entity |
| `automate version-history get <entity-type> <entity-id> <version>` | Get one stored version of an entity (the full payload as it was) |
| `automate version-history list <entity-type> <entity-id>` | List stored versions of an entity (paginated; --skip/--take) |
| `automate version-history types` | List the entity types that keep version history |
| `automate workspace get <id>` | Get a workspace by ID |
| `automate workspace group get <workspace-id> <group-id>` | Get an automation group |
| `automate workspace group list <workspace-id>` | List automation groups in a workspace |
| `automate workspace list` | List workspaces (paginated; --skip/--take/--all) |

### approvals pending

```bash
umbraco automate approvals pending
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### automation ancestors

```bash
umbraco automate automation ancestors <id>
```

### automation export

```bash
umbraco automate automation export <id>
```

GET /automations/{id}/export. The export model is the template format for 'automation validate' and 'automation import' -- export a working automation, adjust the JSON, validate, and import it elsewhere.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--include` | string | — | Export include option |

### automation get

```bash
umbraco automate automation get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### automation list

```bash
umbraco automate automation list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--filter` | string | — | Text filter |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--group-id` | string | — | Group ID |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--workspace-id` | string | — | Workspace ID |

### automation runs

```bash
umbraco automate automation runs <id>
```

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

### automation validate

```bash
umbraco automate automation validate --workspace-id <id> --file <export.json>
```

POST /automations/import/validate. Checks an export model against a workspace -- step aliases, connection references, binding syntax -- and reports success/errors/warnings. The dry-run for authoring: validate before 'automation import'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | — | Path to an export-model JSON file (from 'automation export') |
| `--json` | string | — | Export model as inline JSON |
| `--workspace-id` | string | — | Workspace the definition would be imported into (required) |

### catalogue actions

```bash
umbraco automate catalogue actions
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--workspace-id` | string | — | Scope to one workspace |

### catalogue connection-types

```bash
umbraco automate catalogue connection-types
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### catalogue control-flows

```bash
umbraco automate catalogue control-flows
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### catalogue notification-channels

```bash
umbraco automate catalogue notification-channels
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### catalogue output-schema

```bash
umbraco automate catalogue output-schema <alias>
```

POST /catalogue/step-types/{alias}/output-schema. Steps with hasDynamicOutputSchema=true shape their output by their settings; pass the intended settings via --json to see the fields available for ${...} bindings in later steps.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | string | — | JSON body; defaults to {"settings":{}} |

### catalogue step-types

```bash
umbraco automate catalogue step-types
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--type` | string | — | Step type filter |

### catalogue triggers

```bash
umbraco automate catalogue triggers
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--workspace-id` | string | — | Scope to one workspace |

### catalogue webhook-authenticators

```bash
umbraco automate catalogue webhook-authenticators
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### connection get

```bash
umbraco automate connection get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### connection list

```bash
umbraco automate connection list
```

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

### metrics by-automation

```bash
umbraco automate metrics by-automation
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date (ISO) |
| `--workspace-id` | string | — | Workspace ID |

### metrics summary

```bash
umbraco automate metrics summary
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--to` | string | — | End date (ISO) |
| `--workspace-id` | string | — | Workspace ID |

### run get

```bash
umbraco automate run get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### version-history compare

```bash
umbraco automate version-history compare <entity-type> <entity-id> <from-version> <to-version>
```

### version-history get

```bash
umbraco automate version-history get <entity-type> <entity-id> <version>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### version-history list

```bash
umbraco automate version-history list <entity-type> <entity-id>
```

GET /version-history/{entityType}/{entityId}. entity-type is one of 'version-history types' (e.g. Automation); entity-id is the entity's GUID. The response wraps the versions array with currentVersion/publishedVersion/totalVersions, so it does not follow the standard {items,total} envelope.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### version-history types

```bash
umbraco automate version-history types
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### workspace get

```bash
umbraco automate workspace get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### workspace group get

```bash
umbraco automate workspace group get <workspace-id> <group-id>
```

### workspace group list

```bash
umbraco automate workspace group list <workspace-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### workspace list

```bash
umbraco automate workspace list
```

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

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `automate approvals decide <run-id> <step-id>` | Submit an approval decision for a suspended run step |
| `automate automation create` | Create an automation (draft; publish separately) |
| `automate automation delete <id>` | Permanently delete an automation (including its run history) |
| `automate automation import --workspace-id <id> --file <export.json>` | Import an automation definition as a new automation |
| `automate automation import-update <id> --file <export.json>` | Overwrite an existing automation from an export model |
| `automate automation publish <id>` | Publish the automation's current draft so it goes live |
| `automate automation re-enable <id>` | Re-enable an automation disabled after repeated failures |
| `automate automation trigger <id>` | Trigger a published automation manually |
| `automate automation unpublish <id>` | Unpublish the automation so it stops triggering |
| `automate automation update <id>` | Update an automation |
| `automate connection create` | Create a connection |
| `automate connection delete <id>` | Permanently delete a connection (automations referencing it will fail) |
| `automate connection test <id>` | Test that a connection's credentials work against the external service |
| `automate connection update <id>` | Update a connection |
| `automate run replay <id>` | Replay a run |
| `automate run resume <id>` | Resume a suspended run |
| `automate run suspend <id>` | Suspend a run |
| `automate run terminate <id>` | Terminate a run |
| `automate version-history rollback <entity-type> <entity-id> <version>` | Roll an entity back to a stored version |
| `automate workspace create` | Create a workspace |
| `automate workspace delete <id>` | Permanently delete a workspace |
| `automate workspace group add <workspace-id>` | Add an automation group to a workspace |
| `automate workspace group remove <workspace-id> <group-id>` | Remove an automation group from a workspace |
| `automate workspace group update <workspace-id> <group-id>` | Rename or move an automation group |
| `automate workspace update <id>` | Update a workspace |

### approvals decide

```bash
umbraco automate approvals decide <run-id> <step-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--comment` | string | — | Approval comment |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--outcome` | string | — | Approval outcome: Approved or Rejected |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate approvals decide <run-id> <step-id> --dry-run

# 2. Execute
umbraco automate approvals decide <run-id> <step-id>
```

### automation create

```bash
umbraco automate automation create
```

POST /automations. Required: alias, name, workspaceId (from 'workspace list'), steps (the action sequence), connections (connection GUIDs used by the steps; [] for none). trigger defines what starts the flow; step aliases come from the catalogue commands.

Creating leaves the automation as a draft -- 'automation publish <id>' makes it live. For building from an existing automation, prefer the export/validate/import round-trip.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation create --dry-run

# 2. Execute
umbraco automate automation create
```

### automation delete

```bash
umbraco automate automation delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation delete <id> --dry-run

# 2. Execute
umbraco automate automation delete <id>
```

### automation import

```bash
umbraco automate automation import --workspace-id <id> --file <export.json>
```

POST /automations/import. Creates a new draft automation from an export model. Run 'automation validate' first -- it performs the same checks without writing.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--file` | string | — | Path to an export-model JSON file (from 'automation export') |
| `--json` | string | — | Export model as inline JSON |
| `--workspace-id` | string | — | Workspace to import into (required) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation import --workspace-id <id> --file <export.json> --dry-run

# 2. Execute
umbraco automate automation import --workspace-id <id> --file <export.json>
```

### automation import-update

```bash
umbraco automate automation import-update <id> --file <export.json>
```

PUT /automations/{id}/import. Unlike 'automation import' this takes the bare export model as the body (no workspace wrapper -- the automation already lives somewhere) and replaces the automation's definition with it.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--file` | string | — | Path to an export-model JSON file (from 'automation export') |
| `--json` | string | — | Export model as inline JSON |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation import-update <id> --file <export.json> --dry-run

# 2. Execute
umbraco automate automation import-update <id> --file <export.json>
```

### automation publish

```bash
umbraco automate automation publish <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation publish <id> --dry-run

# 2. Execute
umbraco automate automation publish <id>
```

### automation re-enable

```bash
umbraco automate automation re-enable <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation re-enable <id> --dry-run

# 2. Execute
umbraco automate automation re-enable <id>
```

### automation trigger

```bash
umbraco automate automation trigger <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation trigger <id> --dry-run

# 2. Execute
umbraco automate automation trigger <id>
```

### automation unpublish

```bash
umbraco automate automation unpublish <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation unpublish <id> --dry-run

# 2. Execute
umbraco automate automation unpublish <id>
```

### automation update

```bash
umbraco automate automation update <id>
```

PUT /automations/{id}. The update model requires the automation's current version field for optimistic concurrency; --merge-json picks it up from the fetch automatically, making it the safe default for partial edits (e.g. renaming, tweaking one step's settings).

Updating creates a new draft version; 'automation publish <id>' makes it live.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate automation update <id> --dry-run

# 2. Execute
umbraco automate automation update <id>
```

### connection create

```bash
umbraco automate connection create
```

POST /connections. Required: alias, name, type (from 'catalogue connection-types'), settings (the type's credential fields). Verify it works afterwards with 'connection test'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate connection create --dry-run

# 2. Execute
umbraco automate connection create
```

### connection delete

```bash
umbraco automate connection delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate connection delete <id> --dry-run

# 2. Execute
umbraco automate connection delete <id>
```

### connection test

```bash
umbraco automate connection test <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate connection test <id> --dry-run

# 2. Execute
umbraco automate connection test <id>
```

### connection update

```bash
umbraco automate connection update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate connection update <id> --dry-run

# 2. Execute
umbraco automate connection update <id>
```

### run replay

```bash
umbraco automate run replay <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate run replay <id> --dry-run

# 2. Execute
umbraco automate run replay <id>
```

### run resume

```bash
umbraco automate run resume <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate run resume <id> --dry-run

# 2. Execute
umbraco automate run resume <id>
```

### run suspend

```bash
umbraco automate run suspend <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate run suspend <id> --dry-run

# 2. Execute
umbraco automate run suspend <id>
```

### run terminate

```bash
umbraco automate run terminate <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate run terminate <id> --dry-run

# 2. Execute
umbraco automate run terminate <id>
```

### version-history rollback

```bash
umbraco automate version-history rollback <entity-type> <entity-id> <version>
```

POST /version-history/{entityType}/{entityId}/{entityVersion}/rollback. The undo path after an agent edit goes wrong: pick the version from 'version-history list', confirm with 'get' or 'compare', then roll back.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate version-history rollback <entity-type> <entity-id> <version> --dry-run

# 2. Execute
umbraco automate version-history rollback <entity-type> <entity-id> <version>
```

### workspace create

```bash
umbraco automate workspace create
```

POST /workspaces. Required: alias, name, serviceAccountKey (a backoffice user GUID automations run as), userGroups (who may edit), allowedConnections (connection GUIDs automations here may use; [] for none). Use --print-template for the shape.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace create --dry-run

# 2. Execute
umbraco automate workspace create
```

### workspace delete

```bash
umbraco automate workspace delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace delete <id> --dry-run

# 2. Execute
umbraco automate workspace delete <id>
```

### workspace group add

```bash
umbraco automate workspace group add <workspace-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Group name (required) |
| `--parent-id` | string | — | Parent group ID for nesting |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace group add <workspace-id> --dry-run

# 2. Execute
umbraco automate workspace group add <workspace-id>
```

### workspace group remove

```bash
umbraco automate workspace group remove <workspace-id> <group-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace group remove <workspace-id> <group-id> --dry-run

# 2. Execute
umbraco automate workspace group remove <workspace-id> <group-id>
```

### workspace group update

```bash
umbraco automate workspace group update <workspace-id> <group-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Group name (required) |
| `--parent-id` | string | — | Parent group ID for nesting |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace group update <workspace-id> <group-id> --dry-run

# 2. Execute
umbraco automate workspace group update <workspace-id> <group-id>
```

### workspace update

```bash
umbraco automate workspace update <id>
```

PUT /workspaces/{id}. The update model requires the workspace's current version field for optimistic concurrency; --merge-json picks it up from the fetch automatically.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco automate workspace update <id> --dry-run

# 2. Execute
umbraco automate workspace update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco automate --help

# Inspect a specific endpoint schema
umbraco schema automate.<method>
```
