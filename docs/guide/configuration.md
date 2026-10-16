# Configuration

All kubectl-sheep state lives under your home directory.

## Paths

| Path | Purpose |
|------|---------|
| `~/.config/kubectl-sheep/instances.yaml` | Registered Rancher instances (name, URL, storage, TLS) |
| `~/.config/kubectl-sheep/credentials.plain.yaml` | Plaintext API tokens (`0600`) |
| `~/.config/kubectl-sheep/keys/` | Encrypted tokens (keyring FileBackend) |
| `~/.kube/sheep/<instance>/<cluster-id>.yaml` | Downloaded kubeconfigs |
| `~/.kube/sheep/<instance>/<cluster-id>.meta.yaml` | Fetch metadata (name, timestamp) |
| `~/.kube/config` | Merged contexts (or first path in `$KUBECONFIG`) |

## instances.yaml

Managed by kubectl-sheep — do not edit while commands are running. Example shape:

```yaml
instances:
  - name: prod
    url: https://rancher.example.com
    storage: encrypted
    insecureSkipVerify: false
  - name: dev
    url: https://rancher.dev.example.com
    storage: plaintext
    insecureSkipVerify: false
    auth:                          # set by --auth-login / --ldap-login, used by connect
      providerType: activeDirectory
      providerId: activeDirectory
      username: alice
```

The `auth` block never contains a password; it is prompted for when a token has to be renewed.

## Environment variables

| Variable | Effect |
|----------|--------|
| `KUBECONFIG` | Merge target (first path if colon-separated list) |
| `HOME` | Base for config and kubeconfig paths |

## Global flags

| Flag | Description |
|------|-------------|
| `--no-input` | Disable interactive prompts |
| `-h, --help` | Help with examples on every command |

## Shell completion

```bash
source <(kubectl sheep completion bash)   # or zsh / fish
```

See [Install](install.md#shell-completion) for details.

## Development

```bash
go test ./...
go build -o kubectl-sheep ./cmd/kubectl-sheep
golangci-lint run ./...
```

## Reporting issues

[GitHub Issues](https://github.com/ahoz/kubectl-sheep/issues)

Include: kubectl-sheep version, OS/arch, Rancher version, and the command you ran (redact tokens and URLs if needed).
