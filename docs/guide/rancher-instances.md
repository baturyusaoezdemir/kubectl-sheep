# Rancher instances

The `rancher-instance` command group manages your Rancher connections — URLs, tokens, and storage preferences.

## Add an instance

```bash
kubectl sheep rancher-instance add <name> <url> [flags]
```

| Flag | Description |
|------|-------------|
| `--storage` | `encrypted` (default) or `plaintext` |
| `--insecure` | Skip TLS certificate verification |
| `--open` | Open the Rancher API key page in the browser |
| `--auth-login`, `--auth-username` | Create the token via an auth provider login; the login is stored for later renewals |
| `--ldap-login`, `--ldap-username` | Same for OpenLDAP (see [Authentication](authentication.md)) |

Positional URL (no `--url` flag):

```bash
kubectl sheep rancher-instance add prod https://rancher.example.com
```

Interactive wizard when run with no arguments on a TTY:

```bash
kubectl sheep rancher-instance add
```

## List instances

```bash
kubectl sheep rancher-instance list
```

```
NAME   URL                          STORAGE    INSECURE  AUTH LOGIN
prod   https://rancher.example.com  encrypted  false     activeDirectory (alice)
dev    https://rancher.dev.example  plaintext  true      -
```

## Update token

When a Rancher API token expires:

```bash
kubectl sheep rancher-instance update-token prod
kubectl sheep rancher-instance update-token prod --open
```

With an auth provider login:

```bash
# Renew via Active Directory and store the login for future renewals
kubectl sheep rancher-instance update-token prod --auth-login --auth-username alice

# Instance already has a stored login: only the password is prompted
kubectl sheep rancher-instance update-token prod

# Paste a token although a login is stored
kubectl sheep rancher-instance update-token prod --auth-login=false
```

Interactive — pick the instance:

```bash
kubectl sheep rancher-instance update-token
```

To renew all expired tokens at once, see [Auto-connect at startup](connect.md).

## Change storage mode

Migrate a token between plaintext and encrypted backends:

```bash
kubectl sheep rancher-instance set-storage prod --to=plaintext
kubectl sheep rancher-instance set-storage prod --to=encrypted
```

## List remote clusters

Inventory on the Rancher server (not local kubeconfigs):

```bash
kubectl sheep rancher-instance clusters list prod
```

Interactive — pick the instance:

```bash
kubectl sheep rancher-instance clusters list
```

## Remove an instance

Deletes the instance config and stored credentials:

```bash
kubectl sheep rancher-instance remove prod
```

Interactive — pick the instance:

```bash
kubectl sheep rancher-instance remove
```

## Configuration file

Instance metadata is stored in `~/.config/kubectl-sheep/instances.yaml`. See [Configuration](configuration.md) for all paths.
