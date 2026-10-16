# Auto-connect at startup

`kubectl sheep connect` checks every configured Rancher instance in one go, renews tokens that Rancher rejects, and refreshes the kubeconfigs stored under `~/.kube/sheep/`. It is built to run every time a shell starts, for example in WSL.

```bash
kubectl sheep connect            # all instances
kubectl sheep connect prod dev   # selected instances
```

## What happens per instance

1. The stored Rancher API token is validated against the instance.
2. If Rancher answers `401` (token expired, revoked, or missing):
   - Instances registered with an auth provider login (`--auth-login` / `--ldap-login`) prompt for that **provider password**. The password is asked **once** and reused for every instance that shares the same provider and username, so all Active Directory bound tokens are renewed with a single prompt.
   - Instances bound to a **different directory** (same user name, other password) reject the known password; `connect` names the instance and asks again. Every password that worked is kept for the remaining instances of the run.
   - Instances without a stored login are reported with a hint to run `rancher-instance update-token`.
   - The new token is validated before the old one is replaced. Wrong passwords can be retried up to three times.
3. Unless `--no-refresh` is set, every locally stored kubeconfig is re-fetched and re-merged into `~/.kube/config`.
   Contexts created with `kubeconfig install-exec` load credentials on demand and are left untouched.

Example output after a token expired:

```
  Rancher-instance "prod": token invalid or expired. Log in via activeDirectory (alice) to renew it.
  Password for alice: ********
✓ prod: token renewed via activeDirectory (alice)
✓ prod: connected, 4 kubeconfigs refreshed, 4 updated
✓ dev: token renewed via activeDirectory (alice)
✓ dev: connected, 2 kubeconfigs refreshed, 2 updated
```

| Flag | Description |
|------|-------------|
| `--no-refresh` | Only validate and renew tokens; keep stored kubeconfigs untouched |
| `-q, --quiet` | Print only prompts, renewals, and errors — ideal for shell startup |
| `--no-input` | Never prompt; expired tokens are reported as errors instead |

The command exits non-zero when at least one instance could not be connected.

## Prerequisites

Register (or re-register) each instance with an auth provider login so that `connect` knows how to renew its token:

```bash
# New instance
kubectl sheep rancher-instance add prod https://rancher.example.com \
  --auth-login --auth-username alice

# Existing instance: renew once with --auth-login, the login settings are stored
kubectl sheep rancher-instance update-token prod --auth-login --auth-username alice
```

`rancher-instance list` shows the stored login in the `AUTH LOGIN` column. See [Authentication](authentication.md) for provider types and the LDAP shortcut.

For an unattended startup, prefer `--storage plaintext` for these instances; encrypted storage asks for its passphrase before the token can be checked.

## WSL / Linux shell startup

For a complete WSL + Windows walkthrough (shared Windows kubeconfig, team setup script) see [WSL + Windows setup](wsl-setup.md).

Add one line to `~/.bashrc` (or `~/.zshrc`):

```bash
command -v kubectl-sheep >/dev/null && kubectl sheep connect --quiet
```

With `--quiet`, a healthy setup prints nothing. When a token has expired you are asked for your password right when the shell opens, and every Active Directory bound instance is renewed with it.

> `/etc/wsl.conf` `[boot] command=` runs as root without a terminal, so it cannot prompt for a password. Use the shell startup file of your user instead.

Optional: skip the check in non-interactive shells, so scripts and `scp` stay fast:

```bash
case $- in *i*)
  command -v kubectl-sheep >/dev/null && kubectl sheep connect --quiet ;;
esac
```
