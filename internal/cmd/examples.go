package cmd

// Example blocks for cobra help output. Prefix with "kubectl sheep" as users invoke
// the plugin through kubectl.

const (
	exCompletion = `  # bash
  source <(kubectl sheep completion bash)

  # zsh
  source <(kubectl sheep completion zsh)

  # fish
  kubectl sheep completion fish | source

  # 🪄 Then tab-complete instances and clusters, e.g.:
  kubectl sheep kubeconfig get <TAB>
  kubectl sheep rancher-instance remove <TAB>`

	exRoot = `  # Register a Rancher instance
  kubectl sheep rancher-instance add prod https://rancher.example.com

  # Fetch kubeconfigs (saved under ~/.kube/sheep/ and merged into ~/.kube/config)
  kubectl sheep kubeconfig get prod my-cluster
  kubectl sheep kubeconfig get prod --all

  # Validate tokens, renew expired ones, refresh stored kubeconfigs (e.g. from ~/.bashrc)
  kubectl sheep connect

  # 🐑 Interactive — omit arguments on a TTY
  kubectl sheep rancher-instance add
  kubectl sheep kubeconfig get
  kubectl sheep kubeconfig list

  # 🪄 Shell completion
  source <(kubectl sheep completion bash)`

	exVersion = `  kubectl sheep version`

	exConnect = `  # Check every configured Rancher instance and refresh stored kubeconfigs
  kubectl sheep connect

  # Only selected instances
  kubectl sheep connect prod dev

  # Renew tokens only, keep stored kubeconfigs untouched
  kubectl sheep connect --no-refresh

  # Shell startup (WSL/Linux): add to ~/.bashrc or ~/.zshrc
  command -v kubectl-sheep >/dev/null && kubectl sheep connect --quiet

  # Register an instance so that expired tokens can be renewed with your AD password
  kubectl sheep rancher-instance add prod https://rancher.example.com \
    --auth-login --auth-username alice`

	exRancherInstance = `  kubectl sheep rancher-instance add prod https://rancher.example.com
  kubectl sheep rancher-instance list
  kubectl sheep rancher-instance clusters list prod
  kubectl sheep rancher-instance update-token prod
  kubectl sheep rancher-instance remove prod

  # 🐑 Interactive — pick the instance where supported
  kubectl sheep rancher-instance remove
  kubectl sheep rancher-instance clusters list

  # 🪄 Tab-complete instance names after enabling completion
  kubectl sheep rancher-instance remove <TAB>`

	exRancherInstanceAdd = `  # Register with encrypted token storage
  kubectl sheep rancher-instance add prod https://rancher.example.com --storage=encrypted

  # 🐑 Interactive wizard (name, URL, storage, TLS, browser)
  kubectl sheep rancher-instance add

  # Log in via Rancher auth provider instead of pasting a token
  kubectl sheep rancher-instance add prod https://rancher.example.com \
    --auth-login --auth-username alice

  # Detect the provider (Active Directory, OpenLDAP, FreeIPA) from the instance
  kubectl sheep rancher-instance add prod https://rancher.example.com \
    --auth-login --auth-username alice --auth-provider-type auto --auth-provider-id auto`

	exRancherInstanceList = `  kubectl sheep rancher-instance list`

	exRancherInstanceRemove = `  kubectl sheep rancher-instance remove prod

  # 🐑 Interactive — pick the instance
  kubectl sheep rancher-instance remove`

	exRancherInstanceSetStorage = `  kubectl sheep rancher-instance set-storage prod --to=encrypted

  # 🪄 Tab-complete instance name and --to= value
  kubectl sheep rancher-instance set-storage <TAB> --to=<TAB>`

	exRancherInstanceUpdateToken = `  kubectl sheep rancher-instance update-token prod
  kubectl sheep rancher-instance update-token prod --open

  # Renew through an auth provider; the login settings are stored for "connect"
  kubectl sheep rancher-instance update-token prod --auth-login --auth-username alice

  # Instance registered with --auth-login: only the password is prompted
  kubectl sheep rancher-instance update-token prod

  # Force pasting a token although an auth login is stored
  kubectl sheep rancher-instance update-token prod --auth-login=false

  # 🐑 Interactive — pick the instance
  kubectl sheep rancher-instance update-token`

	exRancherInstanceClusters = `  kubectl sheep rancher-instance clusters list prod

  # 🐑 Interactive — pick the instance
  kubectl sheep rancher-instance clusters list`

	exRancherInstanceClustersList = `  kubectl sheep rancher-instance clusters list prod

  # 🐑 Interactive — pick the instance
  kubectl sheep rancher-instance clusters list`

	exKubeconfig = `  kubectl sheep kubeconfig list prod
  kubectl sheep kubeconfig get prod my-cluster
  kubectl sheep kubeconfig get prod --all
  kubectl sheep kubeconfig refresh prod --all

  # 🐑 Interactive
  kubectl sheep kubeconfig list
  kubectl sheep kubeconfig get

  # 🪄 Tab-complete instance and cluster arguments
  kubectl sheep kubeconfig get <TAB> <TAB>`

	exKubeconfigList = `  kubectl sheep kubeconfig list prod

  # 🐑 Interactive — pick the instance
  kubectl sheep kubeconfig list`

	exKubeconfigGet = `  # Fetch one cluster (auto-merged as <instance>-<cluster>)
  kubectl sheep kubeconfig get prod my-cluster

  # Fetch every cluster on the instance
  kubectl sheep kubeconfig get prod --all

  # 🐑 Interactive — pick instance and scope (one, multiple, or all)
  kubectl sheep kubeconfig get

  # 🪄 Tab-complete
  kubectl sheep kubeconfig get prod <TAB>`

	exKubeconfigRefresh = `  # Refresh one stored kubeconfig
  kubectl sheep kubeconfig refresh prod my-cluster

  # Refresh everything stored for the instance
  kubectl sheep kubeconfig refresh prod --all

  # 🐑 Interactive — pick instance and scope
  kubectl sheep kubeconfig refresh`

	exKubeconfigInstallExec = `  kubectl sheep kubeconfig install-exec prod c-m-abc123

  # 🪄 Tab-complete instance and cluster
  kubectl sheep kubeconfig install-exec <TAB> <TAB>`
)
