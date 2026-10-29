---
name: umbraco-auth
description: "Persistent authentication helpers"
metadata:
  version: 0.5.6
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# auth

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco auth <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `auth list` | List stored auth profiles without exposing secrets |
| `auth status` | Show the current auth/config status without exposing secrets |
| `auth use <profile>` | Set the active stored auth profile |

### list

```bash
umbraco auth list
```

### status

```bash
umbraco auth status
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | bool | false | List command permission requirements for the resolved user context |

### use

```bash
umbraco auth use <profile>
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `auth login` | Store Umbraco API credentials in the user config after verifying them |
| `auth logout` | Remove stored credentials from the user config and delete the cached access token |

### login

```bash
umbraco auth login
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--base-url` | string | — | Umbraco base URL |
| `--basic-auth-shared-secret` | string | — | The environment's Umbraco:CMS:BasicAuth:SharedSecret value, sent so add-on APIs (Deploy, Forms, Engage) get past basic authentication on Umbraco Cloud non-live environments; an empty value removes it |
| `--basic-auth-shared-secret-header` | string | — | Header the shared secret is sent in (default X-Authentication-Shared-Secret) |
| `--client-id` | string | — | Management API client ID |
| `--client-secret` | string | — | Management API client secret |
| `--dry-run` | bool | false | Verify credentials without persisting them |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco auth login [flags] --dry-run

# 2. Execute with the same flags
umbraco auth login [flags]
```

### logout

```bash
umbraco auth logout
```

Removes the client id and secret from the selected user config, and deletes the access-token file cached for it (~/.umbraco/<profile>.token.json for a profile, ~/.umbraco/tokens/ for --config and environment credentials). --token-only deletes just the token file and leaves the config untouched, even a config that does not parse. The token cache reuses one access token across invocations until shortly before it expires; set UMBRACO_NO_TOKEN_CACHE=1 to turn it off.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Preview logout without modifying the user config or the token file |
| `--token-only` | bool | false | Delete only the cached access token; leave the user config untouched |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco auth logout [flags] --dry-run

# 2. Execute with the same flags
umbraco auth logout [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco auth --help

# Inspect a specific endpoint schema
umbraco schema auth.<method>
```
