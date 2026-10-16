# kubectl-sheep

> A kubectl plugin for fetching and managing kubeconfigs from Rancher-managed clusters.

kubectl-sheep connects kubectl to Rancher: register instance URLs, store API tokens securely, download cluster kubeconfigs, refresh them in bulk, and automatically merge contexts into your main kubeconfig.

## Why kubectl-sheep?

Rancher manages many downstream clusters, but getting their kubeconfigs into your daily workflow can be repetitive. kubectl-sheep automates the boring parts:

- **Multiple Rancher instances** — dev, staging, prod, each with its own token storage preference
- **Local cache** — kubeconfigs saved under `~/.kube/sheep/` with fetch metadata
- **Bulk operations** — `kubeconfig get --all` or `kubeconfig refresh --all`
- **Interactive CLI** — arrow-key menus with search; omit arguments on a TTY
- **Exec contexts** — share kubeconfig entries without embedding long-lived tokens

## Quick example

```bash
# 1. Register a Rancher instance
kubectl sheep rancher-instance add prod https://rancher.example.com

# 2. Fetch a kubeconfig and merge it
kubectl sheep kubeconfig get prod my-cluster

# 3. Use the new context
kubectl --context prod-my-cluster get nodes
```

## Command overview

| Group | Purpose |
|-------|---------|
| `rancher-instance` | Add, list, remove instances; update tokens; list remote clusters |
| `kubeconfig` | Get, refresh, list local kubeconfigs; install exec contexts |
| `connect` | Validate all tokens, renew expired ones via auth provider login, refresh stored kubeconfigs |
| `auth exec` | Internal exec credential helper (used by exec contexts) |

Run `kubectl sheep --help` for examples on every command.

## Next steps

- [Install](install.md) the plugin
- Follow the [quick start](quickstart.md) walkthrough
- Read about [interactive mode](interactive.md) and [authentication](authentication.md)
- Set up [auto-connect at startup](connect.md) for WSL and Linux shells

## License

Apache 2.0 — see the [GitHub repository](https://github.com/ahoz/kubectl-sheep).
