# Authentication

kubectl-sheep needs a Rancher API token per instance. You can paste one manually or create it via an auth provider.

## Manual token (default)

`rancher-instance add` and `update-token` print the Rancher API key creation URL:

```
Create a Rancher API key (copy the Bearer Token) at:
  https://rancher.example.com/dashboard/account/create-api-key
```

Paste the token when prompted. Use `--open` to launch the page in your browser.

## Auth provider login

For Rancher setups with Active Directory or LDAP:

```bash
kubectl sheep rancher-instance add prod \
  https://rancher.example.com \
  --auth-login \
  --auth-username alice \
  --auth-provider-type activeDirectory \
  --auth-provider-id activeDirectory
```

`--auth-provider-type` and `--auth-provider-id` default to `activeDirectory`.

### Detecting the provider

Pass `auto` for both and kubectl-sheep reads the instance's public provider list (`/v3-public/authProviders`) and picks the one directory provider that supports password login (Active Directory, OpenLDAP or FreeIPA; Rancher's local accounts only when no directory is enabled):

```bash
kubectl sheep rancher-instance add prod https://rancher.example.com \
  --auth-login --auth-username alice \
  --auth-provider-type auto --auth-provider-id auto
```

The detected provider is stored for the instance. If several directory providers are enabled, the command lists them and asks you to choose. A login against a provider that does not exist on the instance (Rancher answers `500 authconfigs … not found`) is reported together with the providers that would work.

### LDAP shortcut

```bash
kubectl sheep rancher-instance add prod \
  https://rancher.example.com \
  --ldap-login \
  --ldap-username alice
```

`--ldap-login` is equivalent to `--auth-login` with LDAP provider defaults.

### Stored login settings

When an instance is added or its token is updated with `--auth-login` / `--ldap-login`, the provider type, provider ID, and username are stored in `instances.yaml` (never the password). They are used by:

- `rancher-instance update-token <instance>` — prompts only for the password. Pass `--auth-login=false` to paste a token instead.
- `connect` — renews expired tokens of all instances sharing the same login with a single password prompt. See [Auto-connect at startup](connect.md).

`rancher-instance list` shows the stored login in the `AUTH LOGIN` column.

## Token storage

| Mode | Location | Notes |
|------|----------|-------|
| `encrypted` (default) | `~/.config/kubectl-sheep/keys/` | Passphrase-protected keyring; prompted once per process |
| `plaintext` | `~/.config/kubectl-sheep/credentials.plain.yaml` | File mode `0600`; required for non-interactive exec contexts |

Switch at any time:

```bash
kubectl sheep rancher-instance set-storage prod --to=plaintext
```

> Do not commit or share credential files. They stay on the local machine only.

## Invalid token errors

If Rancher returns 401, kubectl-sheep suggests updating the token:

```bash
kubectl sheep rancher-instance update-token <instance>
```

To renew every expired token at once (and refresh stored kubeconfigs), run:

```bash
kubectl sheep connect
```
