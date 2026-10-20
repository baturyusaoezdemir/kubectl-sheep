# WSL + Windows setup

Goal: manage Rancher logins from WSL, use the **same standard kubeconfig** from Windows tools (kubectl, Lens, k9s, …), and have every token checked and renewed automatically when a WSL shell opens. No exec plugin, no kubectl-sheep on Windows.

```
┌──────────── WSL ────────────┐        ┌──────────── Windows ───────────┐
│ ~/.bashrc → kubectl-sheep   │ writes │ C:\Users\<you>\.kube\config      │
│   connect --quiet           │ ─────► │ (plain contexts with tokens)   │
│ asks for AD password only   │        │ kubectl / Lens / k9s read it   │
│ when a token expired        │        │ as usual                       │
└─────────────────────────────┘        └────────────────────────────────┘
```

## One-time setup with the team script

[`hack/wsl-setup.sh`](https://github.com/baturyusaoezdemir/kubectl-sheep/blob/feature/connect-auto-renew/hack/wsl-setup.sh) does everything below in one go. Edit the `INSTANCES` block (or keep the list in a file and pass `INSTANCES_FILE=…`), then run it inside WSL:

```bash
curl -fsSLO https://raw.githubusercontent.com/baturyusaoezdemir/kubectl-sheep/feature/connect-auto-renew/hack/wsl-setup.sh
# edit INSTANCES=( "prod|https://rancher-prod.example.com" … )
bash wsl-setup.sh
```

The script

1. installs a user-local Go toolchain if needed and builds `~/.local/bin/kubectl-sheep`,
2. points `KUBECONFIG` at the Windows kubeconfig (`/mnt/c/Users/<you>/.kube/config`),
3. registers every instance with `--auth-login` (asks for your AD user name per instance and the password once per directory), stores tokens as `plaintext` so startup needs no passphrase,
4. fetches all kubeconfigs (`kubeconfig get <instance> --all`),
5. adds an auto-connect block to `~/.bashrc` (or `~/.zshrc`),
6. runs the first `connect`.

Re-running it is safe: existing instances get `update-token`, nothing is duplicated.

## Manual steps (same result)

```bash
# 1. build
git clone -b feature/connect-auto-renew https://github.com/baturyusaoezdemir/kubectl-sheep.git
cd kubectl-sheep && go build -o ~/.local/bin/kubectl-sheep ./cmd/kubectl-sheep

# 2. share the Windows kubeconfig
export KUBECONFIG=/mnt/c/Users/<you>/.kube/config

# 3. register instances with AD login (password asked once per instance here)
kubectl-sheep rancher-instance add prod   https://rancher-prod.example.com   --storage plaintext --auth-login --auth-username alice
kubectl-sheep rancher-instance add dev    https://rancher-dev.example.com    --storage plaintext --auth-login --auth-username alice
kubectl-sheep rancher-instance add legacy https://rancher-legacy.example.com --storage plaintext --auth-login --auth-username a.lice   # other directory

# 4. fetch kubeconfigs
for i in prod dev legacy; do kubectl-sheep kubeconfig get "$i" --all; done

# 5. append to ~/.bashrc
export PATH="$HOME/.local/bin:$PATH"
export KUBECONFIG="/mnt/c/Users/<you>/.kube/config"
case $- in *i*) command -v kubectl-sheep >/dev/null && kubectl-sheep connect --quiet ;; esac
```

## What happens at every WSL start

`connect --quiet` checks each instance's Rancher API token:

- **All tokens valid** → nothing is printed, the stored kubeconfigs are refreshed silently.
- **A token expired** → you are asked once for the AD password of that login. The password is kept for the rest of the run and tried on every other instance with the same user name, so all instances behind the same directory are renewed with one prompt.
- **An instance belongs to another directory** (same user name, other password) → the known password is rejected, `connect` names the instance and asks again. Both passwords are remembered for the remaining instances.
- **Different user name** on an instance → stored per instance; `connect` prompts for that user separately.

Passwords are never written to disk; only the resulting Rancher API tokens are stored.

## Changing user name or directory for one instance

```bash
kubectl-sheep rancher-instance update-token legacy --auth-login --auth-username new.name
kubectl-sheep rancher-instance list      # AUTH LOGIN column shows the stored login
```

## Windows side

Nothing to install. Windows kubectl reads `%USERPROFILE%\.kube\config` by default, which is exactly the file WSL writes. Contexts are named `<instance>-<cluster>`:

```powershell
kubectl config get-contexts
kubectl --context prod-my-cluster get nodes
```

> Keep `kubeconfig install-exec` out of this setup: exec contexts would require kubectl-sheep on Windows. `connect` never touches exec contexts, so a mixed setup still works, but plain token contexts are what Windows needs here.

## Plain Linux hosts (servers, jump hosts)

The same script works without WSL: it detects the missing Windows profile and uses `~/.kube/config`. The auto-connect block runs in every interactive shell, so an SSH login triggers the token check and, if needed, the password prompt on that terminal.

- Non-interactive SSH commands (`ssh host kubectl get pods`) skip the block, so scripts are never blocked by a prompt.
- Cron or systemd jobs run without a terminal: use `kubectl-sheep connect --no-input` there. An expired token is then reported as an error instead of prompting.
- On distributions whose login shell does not source `~/.bashrc` (SSH gives you a login shell), add `[ -f ~/.bashrc ] && . ~/.bashrc` to `~/.bash_profile` or `~/.profile`.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `✗ prod: token invalid or expired; run: … update-token prod` | The instance has no stored AD login. Run `update-token prod --auth-login --auth-username <user>` once. |
| Prompt appears although the password is right | Another directory or a changed user name. Check with `rancher-instance list`, fix with `update-token <instance> --auth-login --auth-username <user>`. |
| Nothing happens at shell start | The block runs only in interactive shells. Run `kubectl-sheep connect` manually to see output. |
| Windows kubectl shows old contexts | `KUBECONFIG` in WSL must point to the Windows file; check `echo $KUBECONFIG`. |
