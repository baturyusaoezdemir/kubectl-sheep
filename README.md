<p align="center">
  <img src="sheep.png" alt="kubectl-sheep logo" width="160">
</p>

# kubectl-sheep

A [Krew](https://krew.sigs.k8s.io/) plugin for managing kubeconfigs from Rancher-managed clusters.

**Website & docs:** [ahoz.github.io/kubectl-sheep](https://ahoz.github.io/kubectl-sheep) · source in [`docs/`](docs/)

## Install

```bash
kubectl krew install sheep
```

Requires [Krew](https://krew.sigs.k8s.io/) — the kubectl plugin manager.

## Usage

```bash
# Manage Rancher instance connections
kubectl sheep rancher-instance add prod https://rancher.example.com --storage=encrypted
kubectl sheep rancher-instance add prod https://rancher.example.com --open
kubectl sheep rancher-instance add prod https://rancher.example.com --auth-login --auth-username alice
kubectl sheep rancher-instance list
kubectl sheep rancher-instance set-storage prod --to=plaintext
kubectl sheep rancher-instance update-token prod --open
kubectl sheep rancher-instance remove prod

# List clusters on a Rancher instance (remote inventory)
kubectl sheep rancher-instance clusters list prod

# Kubeconfigs — saved under ~/.kube/sheep/ and merged into ~/.kube/config
kubectl sheep kubeconfig list prod
kubectl sheep kubeconfig get prod my-cluster
kubectl sheep kubeconfig get prod --all
kubectl sheep kubeconfig refresh prod my-cluster
kubectl sheep kubeconfig refresh prod --all
kubectl sheep kubeconfig install-exec prod c-m-abc123

# Validate all tokens, renew expired ones with your AD/LDAP password, refresh kubeconfigs
kubectl sheep connect
kubectl sheep connect --quiet        # for shell startup
```

### Interactive mode

When stdin is a TTY, several commands prompt for missing arguments with arrow-key menus (🐑) instead of failing:

| Command | Prompts for |
|---------|-------------|
| `rancher-instance add` | name, URL, storage, TLS skip, open browser, token |
| `rancher-instance remove` | instance |
| `rancher-instance update-token` | instance, token |
| `rancher-instance clusters list` | instance |
| `kubeconfig list` | instance |
| `kubeconfig get` | instance, scope (one / multiple / all), cluster(s) |
| `kubeconfig refresh` | instance, one stored cluster or all |

Pass `--no-input` to disable prompts (for scripts and CI).

```bash
# Fully interactive
kubectl sheep rancher-instance add
kubectl sheep rancher-instance remove
kubectl sheep kubeconfig get
kubectl sheep kubeconfig list
```

### Shell completion

```bash
source <(kubectl sheep completion bash)   # or zsh / fish
kubectl sheep kubeconfig get <TAB>
```

See `kubectl sheep completion --help` for setup examples.

## Authentication

By default, `rancher-instance add` and `rancher-instance update-token` print the Rancher API key
page and prompt for a Bearer Token. Use `--open` to open that page in the
default browser.

For Rancher auth providers that support API login, kubectl-sheep can create the
Rancher API token directly:

```bash
kubectl sheep rancher-instance add prod \
  https://rancher.example.com \
  --auth-login \
  --auth-username alice \
  --auth-provider-type activeDirectory \
  --auth-provider-id activeDirectory
```

`--auth-provider-type` and `--auth-provider-id` default to `activeDirectory`.
For OpenLDAP, use the LDAP shortcut:

```bash
kubectl sheep rancher-instance add prod \
  https://rancher.example.com \
  --ldap-login \
  --ldap-username alice
```

The provider type, ID, and username are stored per instance (never the password).
`rancher-instance update-token prod` then only asks for the password, and
`connect` can renew expired tokens automatically.

## Auto-connect at startup

`kubectl sheep connect` validates the token of every configured instance, renews
rejected tokens through the stored auth provider login, and refreshes all stored
kubeconfigs. One password prompt covers every instance that shares the same
login, so all Active Directory bound tokens are renewed together.

```bash
kubectl sheep connect              # all instances
kubectl sheep connect prod dev     # selected instances
kubectl sheep connect --no-refresh # tokens only
```

Add it to `~/.bashrc` or `~/.zshrc` (WSL or Linux) so it runs whenever a shell opens:

```bash
command -v kubectl-sheep >/dev/null && kubectl sheep connect --quiet
```

With `--quiet`, a healthy setup prints nothing; an expired token triggers the
password prompt right at startup. Instances without a stored login are reported
with a hint to run `rancher-instance update-token`. Exec-based contexts are left
untouched because they load credentials on demand.

## Context names

`kubeconfig get` accepts either a Rancher cluster name or ID. Fetched kubeconfigs are **automatically merged** into `~/.kube/config` with context name `<rancher-instance>-<cluster>`:

```bash
kubectl sheep rancher-instance clusters list prod
kubectl sheep kubeconfig get prod c-m-abc123
kubectl --context prod-my-cluster get nodes
```

Re-fetching overwrites an existing context with the same name.

## Exec kubeconfigs

Use `kubeconfig install-exec` to create a stable kubeconfig context without storing
Rancher-generated Kubernetes credentials in `~/.kube/config`. The merged user
entry uses Kubernetes' exec credential plugin support and calls `kubectl-sheep`
whenever kubectl needs credentials.

```bash
kubectl sheep kubeconfig install-exec prod c-m-abc123
kubectl --context prod-my-cluster get pods
```

The generated kubeconfig user looks like this:

```yaml
users:
- name: prod-my-cluster
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: kubectl-sheep
      interactiveMode: IfAvailable
      args:
      - auth
      - exec
      - prod
      - c-m-abc123
```

Each user still needs to configure their local Rancher instance once with
`rancher-instance add` or `rancher-instance update-token`. After that, the same exec-based
kubeconfig can be shared without embedding tokens.

For non-interactive kubectl calls, make sure the local Rancher token can be read
without a passphrase prompt. For testing, or for environments where the
passphrase-protected file backend is not suitable for exec plugins, use
plaintext storage intentionally:

```bash
kubectl sheep rancher-instance add prod \
  https://rancher.example.com \
  --auth-login \
  --auth-username alice \
  --storage plaintext
```

The plaintext token is stored in `~/.config/kubectl-sheep/credentials.plain.yaml`
with file mode `0600`; do not commit or share that file.

## Configuration

| Path | Purpose |
|------|---------|
| `~/.config/kubectl-sheep/instances.yaml` | Instance names, URLs, storage mode |
| `~/.config/kubectl-sheep/credentials.plain.yaml` | Plaintext tokens (`0600`) |
| `~/.config/kubectl-sheep/keys/` | Encrypted tokens (keyring FileBackend) |
| `~/.kube/sheep/<instance>/<cluster-id>.yaml` | Fetched kubeconfigs |
| `~/.kube/sheep/<instance>/<cluster-id>.meta.yaml` | Fetch metadata |
| `~/.kube/config` | Merged contexts |

## Development

```bash
go test ./...
go build -o kubectl-sheep ./cmd/kubectl-sheep
```

## Credits

This project was herded 🐑 into existence with help from AI tooling 🪄

## License

Apache 2.0
