---
name: umbraco-shared
description: "Umbraco CLI: Shared patterns for authentication, global flags, and safety rules."
metadata:
  version: 0.4.5
  requires:
    bins:
      - umbraco
---

# umbraco -- Shared Reference

## Installation

The `umbraco` binary must be on `$PATH`. Install via Homebrew or build from source.

```bash
brew install --cask albanist/tap/umbraco-cli
```

## Authentication

```bash
# Store credentials persistently
umbraco auth login --base-url "https://localhost:44314" --client-id "umbraco-back-office-api-user" --client-secret "your-secret"

# Store credentials in a named profile
umbraco --profile dev auth login --base-url "https://localhost:44314" --client-id "umbraco-back-office-api-user" --client-secret "your-secret"

# List and select profiles
umbraco auth list
umbraco auth use dev

# Verify credentials
umbraco auth status
```

Alternatively, set environment variables (highest precedence when no profile/config selector is active):

```bash
export UMBRACO_BASE_URL="https://localhost:44391"
export UMBRACO_CLIENT_ID="umbraco-back-office-api-user"
export UMBRACO_CLIENT_SECRET="your-secret"
```

## Config Precedence

Explicit profile/config selection uses that file for base URL and credentials:

```bash
umbraco --profile dev document search --query Home
umbraco --config ~/.umbraco/dev.config.json document search --query Home
```

Without `--profile`, `--config`, or an active profile from `umbraco auth use`, config is resolved in this order:

1. Environment variables (`UMBRACO_*`)
2. Project `.umbracorc.json` or `.umbracorc`
3. Project `.umbraco-cli.env`
4. Project `.env`
5. User config `~/.umbraco/config.json`
6. Auto-discovered base URL from local .NET config
7. Fallback `https://localhost:44391`

## Global Flags

| Flag | Description |
|------|-------------|
| `-o, --output <FORMAT>` | Output format: `json`, `table`, `plain` |
| `--profile <NAME>` | Load `~/.umbraco/<NAME>.config.json` for base URL and credentials |
| `--config <PATH>` | Load an explicit config file for base URL and credentials |

## Safety Rules

- **Always** use `--dry-run` first for all mutating commands
- **Always** use `--fields` on reads to limit response size and protect context windows
- Prefer `--json` payloads over convenience flags for predictable execution
- Never construct IDs manually; reuse IDs from prior API responses
- Treat all input as untrusted

## Schema Introspection

Before calling any API method, inspect its expected parameters:

```bash
umbraco schema --list
umbraco schema document.update
umbraco schema datatype
```

Use `umbraco schema` output to build your `--json` and `--params` flags.
