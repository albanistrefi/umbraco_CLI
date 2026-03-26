---
name: umbraco-cli
description: Use the Umbraco CLI in this repo to inspect and mutate Umbraco Management API resources with agent-safe patterns, schema introspection, dry runs, and compact machine-readable output
version: 1.0.0
location: managed
allowed-tools: Read, Write, Edit, Bash
---

# Umbraco CLI

Use the `umbraco` CLI in this repository when a task involves reading or changing Umbraco Management API resources from the terminal.

This skill is for operating the CLI, not for developing the CLI itself.

## When To Use

Use this skill when the user wants to:

- inspect documents, media, data types, templates, server info, logs, health checks, or dictionary items
- perform Umbraco mutations from the terminal
- discover API shapes or valid payload structure through `umbraco schema ...`
- keep CLI output compact and machine-friendly for agent workflows

Do not use this skill when the task is to change the CLI source code. For that, work directly in the repo.

## Core Rules

- Run `--dry-run` first for mutating commands whenever the command supports it.
- Prefer `--json` and `--params` over convenience flags when the payload is non-trivial.
- Use `--fields` on reads to reduce output size whenever the endpoint supports it.
- Reuse IDs returned by prior commands; do not invent IDs manually.
- Use `--output json` when downstream parsing matters.

## Config Resolution

The CLI resolves config in this order:

1. process env (`UMBRACO_*`)
2. project `.umbracorc.json` or `.umbracorc`
3. project `.umbraco-cli.env`
4. project `.env`
5. user config `~/.umbraco/config.json`
6. local `.NET` URL discovery (`Properties/launchSettings.json`, `appsettings*.json`)
7. fallback base URL

Before assuming auth or base URL are missing, check the local config sources above.

## Workflow

1. **Confirm the CLI is available**
   ```bash
   umbraco --help
   ```

2. **Inspect schema first when payload shape is unclear**
   ```bash
   umbraco schema --list
   umbraco schema document.update
   umbraco schema datatype.search
   ```

3. **Use compact reads to discover IDs or current state**
   ```bash
   umbraco document get <id> --fields "id,name,updateDate" --output json
   umbraco media get <id> --fields "id,name,urls" --output json
   umbraco datatype list --skip 0 --take 50 --output json
   ```

4. **Rehearse writes with `--dry-run`**
   ```bash
   umbraco document publish <id> --json '{"cultures":["en-US"]}' --dry-run --output json
   umbraco datatype add-extension <id> My.Extension --dry-run --output json
   ```

5. **Then run the real command only after the dry run looks correct**

## Common Patterns

### Read Resources

```bash
umbraco document get <id> --fields "id,name,updateDate" --output json
umbraco document children <id> --fields "id,name" --output json
umbraco media get <id> --fields "id,name,urls" --output json
umbraco doctype get <id> --fields "id,name" --output json
umbraco datatype list --skip 0 --take 50 --output json
umbraco logs list --level Error --take 50 --output json
umbraco server status --output json
```

### Search And Discovery

```bash
umbraco document search --params '{"query":"home","skip":0,"take":20}' --output json
umbraco document search --query "home" --under <parent-id> --skip 0 --take 20 --output json
umbraco datatype search --query "rich text" --skip 0 --take 25 --output json
umbraco dictionary list --filter "site." --skip 0 --take 100 --output json
```

### Safe Mutations

```bash
umbraco document update <id> --json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
umbraco document update <id> --json '{"values":[{"alias":"title","value":"New title"}]}' --output json
umbraco document update <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
umbraco document bulk-update --id <id> --id <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json

umbraco datatype update <id> --merge-json '{"configuration":{"toolbar":{"italic":false}}}' --dry-run --output json
umbraco datatype add-value <id> --alias extensions --value My.Extension --dry-run --output json
umbraco datatype add-extension <id> My.Extension --dry-run --output json
umbraco datatype remove-extension <id> My.Extension --dry-run --output json
```

### Datatype Ergonomics

Use these commands instead of hand-editing whole datatype payloads when possible:

- `umbraco datatype extensions <id>`
- `umbraco datatype update <id> --merge-json ...`
- `umbraco datatype add-value <id> --alias <alias> --value <value>`
- `umbraco datatype remove-value <id> --alias <alias> --value <value>`
- `umbraco datatype add-extension <id> <extension-alias>`
- `umbraco datatype remove-extension <id> <extension-alias>`

## Output Guidance

- Use `--output json` when another command, script, or agent step will consume the result.
- `table` is only useful for quick human scanning of simple maps or arrays.
- If you need a small response and the endpoint supports it, combine `--fields` with `--output json`.

## Failure Handling

- If the CLI returns a 404, check `umbraco schema ...` and confirm the target resource or endpoint is valid for the installed Umbraco version.
- If auth fails, inspect resolved config sources before re-exporting credentials.
- If HTTPS fails locally, verify the development certificate trust chain; `NODE_TLS_REJECT_UNAUTHORIZED=0` does not affect this Go CLI.

## Local References

Read these files when you need more detail:

- `README.md` for install, config precedence, and release context
- `CONTEXT.md` for quick command reference and repo-specific CLI conventions
