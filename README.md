# Umbraco CLI (Agent-First)

A Go-based CLI for the Umbraco Management API, designed for **agents first**.

Core behavior:
- `--json` and `--params` are primary machine inputs
- `--fields` keeps responses small for context window discipline
- `--dry-run` previews the planned request for mutating operations before execution
- `umbraco schema ...` provides runtime schema introspection
- JSON output is default when output is piped

## Requirements

- Go `1.26+`
- Node.js `20+` (only needed for skills verification scripts)
- Access to an Umbraco instance with Management API credentials

## Install

### macOS via Homebrew

After the Homebrew tap is in place and a tagged GitHub release is published,
macOS users can install the CLI with a single command:

```bash
brew install --cask albanistrefi/tap/umbraco-cli
umbraco --help
```

If you previously installed from `albanist/tap` (the project moved owners),
re-tap so future upgrades pick up new releases and then clean up the stale tap:

```bash
brew tap albanistrefi/tap
brew upgrade --cask albanistrefi/tap/umbraco-cli
brew untap albanist/tap
```

The Homebrew tap lives at `https://github.com/albanistrefi/homebrew-tap`.

### Build from source

Clone the repository, then run the standard Go workflow:

```bash
go test ./...
go build ./...
```

Run directly with `go run`:

```bash
go run ./cmd/umbraco --help
```

Or build a local binary:

```bash
go build -o ./bin/umbraco ./cmd/umbraco
./bin/umbraco --help
```

## Configure access

Set credentials via environment variables:

```bash
export UMBRACO_BASE_URL="https://localhost:44391"
export UMBRACO_CLIENT_ID="umbraco-back-office-api-user"
export UMBRACO_CLIENT_SECRET="your-secret"
```

Or store credentials persistently once:

```bash
umbraco auth login --base-url "https://localhost:44314" --client-id "umbraco-back-office-api-user" --client-secret "your-secret"
umbraco auth status
```

Use named profiles when you work across multiple environments:

```bash
umbraco --profile dev auth login --base-url "https://dev.example.test" --client-id "umbraco-back-office-api-user" --client-secret "your-secret"
umbraco auth list
umbraco auth use dev
umbraco --profile dev document search --query "Home"
umbraco --config ~/.umbraco/dev.config.json document search --query "Home"
```

Notes:
- Environment variables still work and have the highest precedence when no `--profile`, `--config`, or active profile is selected.
- Project-local `.umbraco-cli.env` files are auto-loaded for `UMBRACO_*` keys and are intended for CLI-specific project setup.
- Project-local `.env` files are auto-loaded for `UMBRACO_*` keys.
- Project-local `.umbracorc.json` or `.umbracorc` can override project defaults.
- User config is read from `~/.umbraco/config.json`.
- If `UMBRACO_BASE_URL` is still unset, the CLI tries local `.NET` config files such as `Properties/launchSettings.json`, `appsettings.Development.json`, and `appsettings.json`.
- `UMBRACO_BASE_URL` should be the site root, for example `https://localhost:44391`, not `https://localhost:44391/umbraco`. The CLI normalizes a trailing `/umbraco` if present.
- Shell profiles such as `.zshrc` are not read.
- Auth/connectivity errors include the resolved base URL so it is obvious what the CLI is trying to reach.

Config precedence, highest to lowest:

Explicit `--profile`, `--config`, or an active profile from `umbraco auth use` selects that config file for base URL and credentials. Without one of those selectors:

1. Process env (`UMBRACO_*`)
2. Project `.umbracorc.json` or `.umbracorc`
3. Project `.umbraco-cli.env`
4. Project `.env`
5. User config `~/.umbraco/config.json`
6. Auto-discovered base URL from local `.NET` config
7. Final fallback `https://localhost:44391`

Example project-local CLI env:

```bash
cp .env.example .umbraco-cli.env
```

Example user config:

```json
{
  "baseUrl": "https://localhost:44314",
  "clientId": "umbraco-back-office-api-user",
  "clientSecret": "your-secret",
  "outputFormat": "json"
}
```

### Local HTTPS trust

If your local Umbraco instance uses HTTPS with a development or self-signed
certificate, the Go CLI must trust that certificate. For local ASP.NET/Umbraco
setups, the usual fix is:

```bash
dotnet dev-certs https --trust
```

If you are not using `.NET` dev certificates, trust the certificate with your
OS trust store or use a certificate issued by a trusted local CA. For example,
`mkcert` is a common option for non-.NET local development setups.

`NODE_TLS_REJECT_UNAUTHORIZED=0` does not affect this CLI because it is a Go
binary, not a Node.js process.

## Release

Tagging a release publishes GitHub release archives and updates the Homebrew
cask in the dedicated tap repository `albanistrefi/homebrew-tap`:

```bash
git tag v0.4.1
git push origin v0.4.1
```

The release workflow uses GoReleaser and expects to run in GitHub Actions.

GitHub release assets are downloaded anonymously by Homebrew, so the source
release repository must stay public — or the cask must be configured with
authenticated download headers.

## First Commands

The examples below assume the installed binary on your `PATH` (`umbraco …`).
If you are working from a source checkout, substitute `go run ./cmd/umbraco …`
in every command.

Schema introspection:

```bash
umbraco schema --list
umbraco schema document.create
umbraco schema doctype.create --template
umbraco schema document
```

Auth helpers:

```bash
umbraco auth login --base-url "https://localhost:44314" --client-id "umbraco-back-office-api-user" --client-secret "your-secret"
umbraco auth list
umbraco auth use dev
umbraco auth status
umbraco auth status --check
umbraco auth logout --dry-run
```

Safe read:

```bash
umbraco document get <id> --fields "id,name,updateDate"
umbraco document get <id> --with-urls --fields "id,name,urls"
umbraco document urls <id> --absolute --output plain
umbraco document search --query "Toxic" --skip 0 --take 25 --output json
umbraco document search --query "Toxic" --under <parent-id> --skip 0 --take 25 --output json
umbraco media search --query "Hero" --skip 0 --take 25 --output json
umbraco doctype root --summarize --first-n 10 --output json
umbraco doctype list --recursive --types-only --fields id,name,alias
umbraco tree walk "Home/Partners/Partner List" --output json
```

Raw Management API passthrough for endpoints that do not have curated commands yet:

```bash
umbraco api GET "/item/document/ancestors?id=<guid>&id=<guid>" --output json
umbraco api POST /some/endpoint --body @payload.json --dry-run --output json
```

Safe write pattern (always dry-run first):

```bash
umbraco document publish <id> --json '{"cultures":["en-US"]}' --dry-run --output json
umbraco document update <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
umbraco document update <id> --property skills --value 'C#;Go' --dry-run --output json
umbraco document update <id> --property skills --value 'C#;Go' --save-and-publish --culture en-US --dry-run --output json
umbraco document copy <id> --to <parent-id> --publish --dry-run --output json
umbraco document bulk-update --id <id> --id <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
umbraco document csv-update --file partners.csv --property skills --dry-run --output json
# then run without --dry-run
```

Create payload discovery:

```bash
umbraco doctype create --print-template
umbraco datatype create --print-template
umbraco media upload ./hero.svg --name "Hero" --type SVG --parent <media-parent-id> --dry-run --output json
umbraco media upload ./hero.png --name "Hero" --type Image --culture en-US --dry-run --output json
```

`media upload --type` accepts a media type ID or an existing media type alias/name; names and aliases are resolved from the live media type list before media creation. Use `--culture` when the media type varies by culture.

Datatype discovery and ergonomic updates:

```bash
umbraco datatype list --skip 0 --take 50
umbraco datatype search --query "rich text" --skip 0 --take 25
umbraco datatype search --editor-alias Umbraco.TextBox --skip 0 --take 25
umbraco datatype extensions <id>
umbraco datatype update <id> --merge-json '{"configuration":{"toolbar":{"italic":false}}}' --dry-run
umbraco datatype add-extension <id> UmbracoDotCom.Tiptap.GoogleDocsPasteCleanup --dry-run
umbraco datatype remove-extension <id> UmbracoDotCom.Tiptap.GoogleDocsPasteCleanup --dry-run
umbraco datatype add-value <id> --alias extensions --value Custom.Extension --dry-run
```

Version history, webhooks, languages, and users (added in v0.4.0):

```bash
umbraco document version list <document-id> --fields id,versionDate --take 10
umbraco document version rollback <version-id> --dry-run
umbraco document audit-log <id> --fields timestamp,logType --take 20
umbraco webhook events --fields alias,eventName
umbraco webhook create --print-template
umbraco webhook logs --fields date,eventAlias,statusCode,retryCount --take 20
umbraco language list --fields isoCode,name,isDefault
umbraco language create --iso-code da-DK --name "Danish" --dry-run
umbraco user current --fields name,userName
umbraco user permissions --ids <node-id> --type document
umbraco user client-credentials create <user-id> --client-id umbraco-back-office-ci --client-secret <secret> --dry-run
```

## Skills Bundle

This repo ships two sets of SKILL.md files under `skills/`:

- **67 bundled Umbraco extension-development skills** (`skills/foundation/`, `skills/backend/`, `skills/extensions/`, `skills/property-editors/`, `skills/rich-text/`, `skills/testing/`) — copied from `.agents/skills/` by `npm run bundle:skills`.
- **26 CLI command skills** (`skills/cli/`) — generated from the cobra command tree by `umbraco generate-skills`.

Verify both sets are present and consistent with the package version:

```bash
npm run verify:skills
```

**Heads-up for Homebrew users:** the cask currently installs only the `umbraco`
binary. The bundled skills are not extracted to disk by `brew install`. To use
them with an agent harness — Claude Code, Codex CLI, Cursor, or any other tool
that reads SKILL.md files — clone this repo and point the harness at the local
`skills/` tree (each harness has its own conventional location, e.g.
`~/.claude/skills/`, `~/.codex/skills/`, project-local `.claude/skills/`):

```bash
git clone https://github.com/albanistrefi/umbraco_CLI.git
# then configure your agent harness with the cloned skills/ path
```

An `umbraco skills install --target <dir>` command that extracts the bundled
skills into whichever harness directory you point it at is on the roadmap.

## Project Commands

- `go test ./...` - run tests
- `go build ./...` - build all packages
- `go run ./cmd/umbraco ...` - run CLI
- `npm run bundle:skills` - copy curated extension-development skills from `.agents/skills/` into `skills/`
- `npm run verify:skills` - verify skills count, structure, and version parity
- `npm run sync:version` - propagate `internal/version/VERSION` into `package.json` / `package-lock.json`

## Collections

- `document` (29) — incl. `urls`, `version` history/rollback, `audit-log`, `publish-descendants`, `sort`, `domains`, `public-access`
- `media` (15)
- `doctype` (12)
- `datatype` (14)
- `dictionary` (6)
- `template` (6)
- `member` (8) / `member-group` (2)
- `user` (13) / `user-group` (7)
- `webhook` (7)
- `language` (7)
- `forms` (6, read-only)
- `models-builder` (3)
- `logs` (5)
- `server` (5)
- `health` (4)
- `tree` (1)
- `api` (1)
- `auth` (5)
- `automate` (54) — requires [Umbraco Automate](https://docs.umbraco.com/umbraco-automate) on the target instance; see below

Total: **221 runnable commands** counting every nested subcommand. Group counts above are direct subcommands; nested subgroups like `document version` and `automate workspace group` add the rest.

## Umbraco Automate

The `automate` command group covers the full [Umbraco Automate](https://docs.umbraco.com/umbraco-automate)
Management API: catalogue discovery, automation authoring (create/update/publish,
plus an export → validate → import round-trip), run control, approvals,
workspaces, connections, version history with rollback, and metrics. It
requires Umbraco Automate to be installed on the target instance.

```bash
umbraco automate workspace list
umbraco automate automation list --fields id,name
umbraco automate catalogue operators
umbraco automate automation export <id> > automation.json
umbraco automate automation validate --workspace-id <ws> --file automation.json
umbraco automate automation import-update <id> --file automation.json --dry-run
umbraco automate automation runs <id> --take 10
```

## Agent Safety Rules

- Use `--dry-run` first for all mutating commands; it prints the planned request without executing.
- Use `--fields` on reads to limit response size.
- Updates follow one contract everywhere: `--json` is a **full replacement** (the server resets unmentioned fields), `--merge-json` fetches the current resource and **deep-merges** your patch. Use `--merge-json` for partial edits.
- Hard deletes require `--force` (or `--dry-run` to rehearse); recycle-bin moves (`trash`) do not.
- Let the CLI generate IDs — every `create` does this automatically and echoes the new id back; reuse that id for subsequent operations.
- Check permissions before destructive runs: `umbraco user permissions --ids <id> --type document`.
