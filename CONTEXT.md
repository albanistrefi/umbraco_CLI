# Umbraco CLI Context

## What This Is

`umbraco` is an agent-first command-line wrapper around the Umbraco Management API.
Implementation runtime is Go (`cmd/umbraco`).

## Core Principles

- Primary input path is raw JSON (`--json`) and structured query JSON (`--params`).
- Partial datatype updates can use `--merge-json` to fetch, merge, and send a full payload automatically.
- Schema is introspectable at runtime (`umbraco schema ...`).
- Response size should be constrained (`--fields`) to protect context window budget.
- Mutations must be rehearsed first (`--dry-run` previews the planned request without executing).
- Updates follow one contract everywhere: `--json` replaces the resource wholesale, `--merge-json` fetches and deep-merges. Hard deletes require `--force` or `--dry-run`.
- Config resolves from an explicit `--profile`/`--config` or active profile first; otherwise env, project config, `.umbraco-cli.env`, `.env`, user config, and local `.NET` URL discovery in that order.
- Auth/connectivity errors include the resolved base URL to make misconfiguration obvious.

## Quick Command Reference

### Auth
- `umbraco auth login --base-url "https://localhost:44314" --client-id "..." --client-secret "..."`
- `umbraco --profile dev auth login --base-url "https://localhost:44314" --client-id "..." --client-secret "..."`
- `umbraco auth list`
- `umbraco auth use dev`
- `umbraco auth status`
- `umbraco auth logout --dry-run`

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
- `umbraco document csv-update --file partners.csv --property skills --dry-run`

### Media
- `umbraco media get <id>`
- `umbraco media root`
- `umbraco media children <id>`
- `umbraco media search --query "hero"`

### Tree
- `umbraco tree walk "Home/Partners/Partner List"`

### Schema
- `umbraco doctype get <id>`
- `umbraco doctype list --recursive --types-only --fields id,name,alias`
- `umbraco datatype list --skip 0 --take 50`
- `umbraco datatype search --query "rich text"`
- `umbraco datatype extensions <id>`
- `umbraco datatype add-extension <id> My.Extension --dry-run`
- `umbraco datatype remove-extension <id> My.Extension --dry-run`
- `umbraco schema document.update`

### Versions, webhooks, languages, users
- `umbraco document version list <documentId>` / `umbraco document version rollback <versionId>`
- `umbraco document audit-log <id>`
- `umbraco webhook list` / `umbraco webhook create --json '{...}'` / `umbraco webhook logs [id]`
- `umbraco language list` / `umbraco language cultures` / `umbraco language create --iso-code da-DK --name Danish`
- `umbraco user list` / `umbraco user current` / `umbraco user permissions --ids <id> --type document`
- `umbraco user client-credentials create <userId> --client-id ... --client-secret ...`
- `umbraco user-group list`

### Diagnostics
- `umbraco api GET "/item/document/ancestors?id=<id>&id=<id>"`
- `umbraco server status`
- `umbraco logs list --level Error --take 50`
- `umbraco logs search --around 2026-06-23T11:38:51Z --minutes 5 --source-context My.Source --flat --redact-default`
- `umbraco health groups`

## Local Dev Commands

- Build: `go build ./...`
- Test: `go test ./...`
- Run: `go run ./cmd/umbraco --help`
