# Umbraco CLI (Agent-First)

A Go-based CLI for the Umbraco Management API, designed for **agents first, humans second**.

Core behavior:
- `--json` and `--params` are primary machine inputs
- `--fields` keeps responses small for context window discipline
- `--dry-run` validates mutating operations before execution
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
brew install --cask albanist/tap/umbraco-cli
umbraco --help
```

Important:
- The Homebrew tap lives at `https://github.com/albanist/homebrew-tap`.
- GitHub release assets are downloaded anonymously by Homebrew. The source
  release repository must therefore be public, or the cask must be configured
  with authenticated download headers.

### Build from source

Clone the repository and enter the project directory.

## Setup

1. Configure access:

```bash
export UMBRACO_BASE_URL="https://localhost:44391"
export UMBRACO_CLIENT_ID="umbraco-back-office-api-user"
export UMBRACO_CLIENT_SECRET="your-secret"
```

Notes:
- Environment variables still work and have the highest precedence.
- Project-local `.umbraco-cli.env` files are auto-loaded for `UMBRACO_*` keys and are intended for CLI-specific project setup.
- Project-local `.env` files are auto-loaded for `UMBRACO_*` keys.
- Project-local `.umbracorc.json` or `.umbracorc` can override project defaults.
- User config is read from `~/.umbraco/config.json`.
- If `UMBRACO_BASE_URL` is still unset, the CLI tries local `.NET` config files such as `Properties/launchSettings.json`, `appsettings.Development.json`, and `appsettings.json`.
- `UMBRACO_BASE_URL` should be the site root, for example `https://localhost:44391`, not `https://localhost:44391/umbraco`. The CLI normalizes a trailing `/umbraco` if present.
- Shell profiles such as `.zshrc` are not read.
- Auth/connectivity errors now include the resolved base URL so it is obvious what the CLI is trying to reach.

Config precedence, highest to lowest:

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

2. Build and test:

```bash
go test ./...
go build ./...
```

3. Run directly:

```bash
go run ./cmd/umbraco --help
```

Optional binary build:

```bash
go build -o ./bin/umbraco ./cmd/umbraco
./bin/umbraco --help
```

## Release

Tagging a release publishes GitHub release archives and updates the Homebrew
cask in the dedicated tap repository `albanist/homebrew-tap`:

```bash
git tag v0.2.2
git push origin v0.2.2
```

The release workflow uses GoReleaser and expects to run in GitHub Actions.

## First Commands

Schema introspection:

```bash
go run ./cmd/umbraco schema --list
go run ./cmd/umbraco schema document.create
go run ./cmd/umbraco schema document
```

Safe read:

```bash
go run ./cmd/umbraco document get <id> --fields "id,name,updateDate"
go run ./cmd/umbraco document search --query "Toxic" --skip 0 --take 25 --output json
go run ./cmd/umbraco document search --query "Toxic" --under <parent-id> --skip 0 --take 25 --output json
go run ./cmd/umbraco media search --query "Hero" --skip 0 --take 25 --output json
```

Safe write pattern (always dry-run first):

```bash
go run ./cmd/umbraco document publish <id> --json '{"cultures":["en-US"]}' --dry-run --output json
go run ./cmd/umbraco document update <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
go run ./cmd/umbraco document update <id> --property skills --value 'C#;Go' --dry-run --output json
go run ./cmd/umbraco document update <id> --property skills --value 'C#;Go' --save-and-publish --culture en-US --dry-run --output json
go run ./cmd/umbraco document bulk-update --id <id> --id <id> --merge-json '{"values":[{"alias":"title","value":"New title"}]}' --dry-run --output json
# then run without --dry-run
```

Datatype discovery and ergonomic updates:

```bash
go run ./cmd/umbraco datatype list --skip 0 --take 50
go run ./cmd/umbraco datatype search --query "rich text" --skip 0 --take 25
go run ./cmd/umbraco datatype extensions <id>
go run ./cmd/umbraco datatype update <id> --merge-json '{"configuration":{"toolbar":{"italic":false}}}' --dry-run
go run ./cmd/umbraco datatype add-extension <id> UmbracoDotCom.Tiptap.GoogleDocsPasteCleanup --dry-run
go run ./cmd/umbraco datatype remove-extension <id> UmbracoDotCom.Tiptap.GoogleDocsPasteCleanup --dry-run
go run ./cmd/umbraco datatype add-value <id> --alias extensions --value Custom.Extension --dry-run
```

## Skills Bundle

This repo includes 67 bundled Umbraco skills under `skills/`.

Verify bundle integrity:

```bash
npm run verify:skills
```

## Project Commands

- `go test ./...` - run tests
- `go build ./...` - build all packages
- `go run ./cmd/umbraco ...` - run CLI
- `npm run verify:skills` - verify skills count and structure

## Collections in MVP

- `document` (16)
- `dictionary` (6)
- `media` (10)
- `doctype` (10)
- `datatype` (13)
- `template` (6)
- `logs` (5)
- `server` (5)
- `health` (4)

Total: **75 commands**

## Agent Safety Rules

- Use `--dry-run` first for all mutating commands.
- Use `--fields` on reads to limit response size.
- Prefer `--json` payloads to avoid lossy argument mapping.
- Do not construct IDs manually; reuse IDs returned by API responses.
