# Umbraco CLI Context

## What This Is

`umbraco` is an agent-first command-line wrapper around the Umbraco Management API.
Implementation runtime is Go (`cmd/umbraco`).

## Core Principles

- Primary input path is raw JSON (`--json`) and structured query JSON (`--params`).
- Partial datatype updates can use `--merge-json` to fetch, merge, and send a full payload automatically.
- Schema is introspectable at runtime (`umbraco schema ...`).
- Response size should be constrained (`--fields`) to protect context window budget.
- Mutations must be rehearsed first (`--dry-run`).
- Config resolves from env, project config, `.umbraco-cli.env`, `.env`, user config, and local `.NET` URL discovery in that order.
- Auth/connectivity errors include the resolved base URL to make misconfiguration obvious.

## Quick Command Reference

### Content
- `umbraco document get <id>`
- `umbraco document root`
- `umbraco document children <id>`
- `umbraco document search --params '{"query":"home"}'`
- `umbraco document search --query "home" --under <parent-id>`
- `umbraco document update <id> --property skills --value 'C#;Go' --dry-run`
- `umbraco document update <id> --property skills --value 'C#;Go' --save-and-publish --culture en-US --dry-run`
- `umbraco document update <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run`
- `umbraco document bulk-update --id <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run`

### Media
- `umbraco media get <id>`
- `umbraco media root`
- `umbraco media children <id>`
- `umbraco media search --query "hero"`

### Schema
- `umbraco doctype get <id>`
- `umbraco datatype list --skip 0 --take 50`
- `umbraco datatype search --query "rich text"`
- `umbraco datatype extensions <id>`
- `umbraco datatype add-extension <id> My.Extension --dry-run`
- `umbraco datatype remove-extension <id> My.Extension --dry-run`
- `umbraco schema document.update`

### Diagnostics
- `umbraco server status`
- `umbraco logs list --level Error --take 50`
- `umbraco health groups`

## Local Dev Commands

- Build: `go build ./...`
- Test: `go test ./...`
- Run: `go run ./cmd/umbraco --help`
