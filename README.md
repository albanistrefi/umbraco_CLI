# Umbraco CLI (Agent-First)

The agent-first command line for Umbraco — CMS, Forms, and Automate. Built on
the Management APIs, it goes beyond them: exhaustive content search,
cross-environment schema diff, log tailing, and safe, rehearsable bulk
operations — 263 commands with a scriptable exit-code contract.

Core behavior:
- `--json` and `--params` are primary machine inputs
- `--fields` keeps responses small for context window discipline
- `--dry-run` previews the planned request for mutating operations before execution
- `umbraco schema ...` provides runtime schema introspection
- JSON output is default when output is piped

## Requirements

- Go `1.26+`
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

### Development checks

CI enforces formatting, `go vet`, golangci-lint, the race detector, and a total
test-coverage floor. Run the same checks locally before pushing:

```bash
gofmt -l .
go vet ./...
golangci-lint run ./...   # brew install golangci-lint
go test -race ./...
```

The lint rules live in `.golangci.yml`; the coverage floor is set in
`.github/workflows/ci.yml` and is ratcheted up as coverage grows.

## Shell completions

Cobra-generated completions ship built in:

```bash
umbraco completion zsh > "${fpath[1]}/_umbraco"   # zsh
umbraco completion bash > /usr/local/etc/bash_completion.d/umbraco
umbraco completion fish > ~/.config/fish/completions/umbraco.fish
```

Run `umbraco completion <shell> --help` for shell-specific install notes.

## Exit codes

Scripts and CI gates can rely on the exit code to tell failure classes apart:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Usage or local error (invalid flags, bad payloads, missing files) |
| 2 | `schema diff` ran cleanly and found differences (suppress with `--exit-zero`) |
| 3 | Authentication or credential failure (fix credentials/base URL, not the command) |
| 4 | The Management API answered with an error status (4xx/5xx) |
| 5 | `deploy watch` reached its failed phase (sustained downtime or post-landing health failure beyond `--escalation`) |
| 6 | `deploy watch` reached `--timeout` without verification — deployment status unknown, never inferred |
| 7 | `deploy status` ran cleanly and found drifted or missing entities (suppress with `--exit-zero`) |

`deploy apply` uses exit 4 for a failed or still-drifting write (it is an API-level failure), never exit 7.

JSON output is the stable machine contract — only additive changes. Table
output is unstable and may reorder columns between releases.

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
umbraco api POST /temporary-file --form id=<uuid> --form file=@./logo.svg --output json          # multipart instead of JSON
umbraco api GET /umbraco/automate/management/api/v1/automations --raw-path --output json         # any path on the host
umbraco api GET /server/status --header "X-Trace: abc" --output json                            # extra/override headers
umbraco api GET /media/abc123/logo.png --raw-path --out ./logo.png --output json                 # save a binary response verbatim
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

```bash
umbraco media inspect <media-id> --output json                                    # name, type, file src/url, extension, bytes, width/height or viewBox
umbraco media references <media-id> --output json                                 # which content uses this item (aliases: find-references, referenced-by, usage)
umbraco media download <media-id> ./assets/ --output json                          # save the file verbatim (server file name inside a directory)
umbraco media replace-file <media-id> ./new-logo.svg --backup --output json      # swap the file, keep every other value, verify
umbraco media restore-backup ./media-<id>-<timestamp>.backup.json --output json   # undo: re-uploads the saved binary
umbraco media update <media-id> --merge-json '{...}' --backup=./before.json         # any update command accepts --backup
```

`media replace-file` fetches the item, uploads the new file, rewrites the file property (default `umbracoFile`) and re-reads the item afterwards: a PUT whose temporary file id does not resolve is accepted by the server with 200 while leaving the item with **no values at all**, so the command fails instead of reporting success. `--backup` also downloads the current binary (Umbraco deletes the replaced file), and `restore-backup` re-uploads it; a metadata-only backup (from `update --backup`) is refused when its file is gone rather than restoring a dead reference.

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

## CLI Skills

`skills/cli/` holds SKILL.md files generated from the cobra command tree by
`umbraco generate-skills`; CI fails if they drift from the code. Point an
agent harness (Claude Code, Codex CLI, Cursor, or anything that reads
SKILL.md files) at that directory, or regenerate them anywhere with an
installed binary:

```bash
umbraco generate-skills --output-dir .claude/skills   # or .agents/skills, etc.
```

An `umbraco skills install --target <dir>` convenience command is on the
roadmap. Umbraco extension-development skills (backoffice, property editors,
testing) are not part of this repo — get those from
`umbraco/Umbraco-CMS-Backoffice-Skills`.

## Project Commands

- `go test ./...` - run tests
- `go build ./...` - build all packages
- `go run ./cmd/umbraco ...` - run CLI
- `go run ./cmd/umbraco generate-skills` - regenerate `skills/cli/` after command changes

## Collections

- `document` (30) — incl. `urls`, `version` history/rollback, `audit-log`, `publish-descendants`, `sort`, `domains`, `public-access`, and the `bin` recycle-bin subgroup
- `element` (21) — the Umbraco 18.1+ element library: CRUD with atomic `create --publish`/`update --save-and-publish`, publish lifecycle, `version` history/rollback, references, and the `bin` recycle-bin subgroup
- `media` (23) — incl. `inspect`, `download`, `upload`, `replace-file`/`restore-backup`, `references`, `restore` and the `bin` recycle-bin subgroup
- `doctype` (14) / `mediatype` (8) / `membertype` (8) — the full schema type family, all with `--recursive --types-only` folder handling
- `datatype` (14)
- `dictionary` (6)
- `template` (6)
- `member` (8) / `member-group` (2)
- `user` (13) / `user-group` (7)
- `webhook` (7)
- `language` (7)
- `forms` (6, read-only)
- `models-builder` (3)
- `logs` (6) — incl. `tail` for following new entries as they arrive
- `server` (5)
- `health` (4)
- `deploy` (3) — effect-based deployment observation plus schema application: `watch` polls an environment for state deltas only a deploy can cause (app recycle via ProcessId, 503→401→200 recovery, index rebuilds) and emits phase transitions; `status` compares local Deploy `.uda` artifacts against the environment per entity as a pre-flight drift check; `apply` is its write side — Deploy's "Update schema" from the CLI: creates missing entities with their artifact GUIDs and full-replaces drifted ones in dependency order, backs up every updated entity, re-verifies each write, and refuses to run without `--dry-run` or `--force`
- `published-cache` (3) — status / rebuild / reload, for stale-content incident response
- `indexer` (3) — Examine index health and rebuilds, with `--wait` polling
- `redirect` (6) — the redirect URL tracker: list/get/delete, status, enable/disable
- `tree` (1)
- `api` (1)
- `auth` (5)
- `schema` — runtime schema introspection (`umbraco schema <command>`) plus `schema diff <envA> <envB>` cross-environment comparison across doctype, datatype, mediatype, membertype, template, language, and dictionary
- `automate` (8 subgroups) — requires [Umbraco Automate](https://docs.umbraco.com/umbraco-automate) on the target instance; see below

Total: **263 runnable commands** counting every nested subcommand. Group counts above are direct subcommands; nested subgroups like `document version`, `document bin`, and the `automate` subgroups add the rest.

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
