#!/usr/bin/env bash
#
# kubectl-sheep — WSL / Linux team setup
#
# Installs kubectl-sheep, registers the team's Rancher instances with an
# Active Directory login, fetches every kubeconfig and makes
# "kubectl-sheep connect" run whenever an interactive shell opens.
#
# In WSL the kubeconfig is written to the *Windows* kubeconfig, so kubectl,
# Lens, k9s … on Windows and in WSL share one standard file. On a plain Linux
# host (workstation, jump host, server) it is ~/.kube/config.
#
# Re-running the script is safe: existing instances are updated, not duplicated.
#
# Usage:
#   bash wsl-setup.sh                 # interactive (asks for AD user names)
#   AD_USER=alice bash wsl-setup.sh   # preset the default user name
#
# Hosts without internet access (servers, jump hosts):
#   bash wsl-setup.sh bundle          # on a machine WITH internet (e.g. WSL):
#                                     #   builds a static linux binary and packs it with this
#                                     #   script into kubectl-sheep-offline-<arch>.tar.gz
#   scp kubectl-sheep-offline-amd64.tar.gz server:
#   ssh server 'tar xzf kubectl-sheep-offline-amd64.tar.gz && bash kubectl-sheep-offline/wsl-setup.sh'
#   The script then installs the bundled binary instead of building; only the
#   Rancher instances must be reachable from that host.
#   SHEEP_BINARY=/path/to/kubectl-sheep bash wsl-setup.sh   # same, with any prebuilt binary
#
set -euo pipefail

# ─── Team configuration — edit this block ────────────────────────────────────
# One line per Rancher instance:  name|url[|provider-type|provider-id]
# Without provider fields the login provider is detected from the instance
# (Active Directory, OpenLDAP, FreeIPA or local). Set it explicitly only when an
# instance has several directory providers, e.g. "legacy|https://…|openLdap|openldap".
INSTANCES=(
  "prod|https://rancher-prod.example.com"
  "dev|https://rancher-dev.example.com"
  "legacy|https://rancher-legacy.example.com"
)

# Alternatively keep the list in a file (one name|url[|type|id] per line, # comments
# allowed) and point INSTANCES_FILE at it — handy for sharing with the team.
if [ -n "${INSTANCES_FILE:-}" ] && [ -f "$INSTANCES_FILE" ]; then
  mapfile -t INSTANCES < <(grep -Ev '^\s*(#|$)' "$INSTANCES_FILE")
fi

# Where the kubectl-sheep build comes from.
REPO_URL="${REPO_URL:-https://github.com/baturyusaoezdemir/kubectl-sheep.git}"
REPO_REF="${REPO_REF:-feature/connect-auto-renew}"

# Token storage. plaintext (0600 file) lets "connect" run at shell start without
# a passphrase prompt. Use encrypted if your policy requires it.
STORAGE="${STORAGE:-plaintext}"
# ─────────────────────────────────────────────────────────────────────────────

GO_VERSION="${GO_VERSION:-1.26.0}"
BIN_DIR="$HOME/.local/bin"
SRC_DIR="$HOME/.local/src/kubectl-sheep"
SHEEP="$BIN_DIR/kubectl-sheep"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

say()  { printf '\n\033[1m%s\033[0m\n' "$*"; }
note() { printf '  %s\n' "$*"; }
die()  { printf '\n\033[31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# A prebuilt binary next to this script (offline bundle) or via SHEEP_BINARY
# skips git, Go and every download.
PREBUILT="${SHEEP_BINARY:-}"
if [ -z "$PREBUILT" ] && [ -x "$SCRIPT_DIR/kubectl-sheep" ]; then
  PREBUILT="$SCRIPT_DIR/kubectl-sheep"
fi

# ─── 0. Preconditions ────────────────────────────────────────────────────────
IS_WSL=0
grep -qi microsoft /proc/version 2>/dev/null && IS_WSL=1
[ "$IS_WSL" = 1 ] || note "Plain Linux detected (no WSL): the kubeconfig stays at ~/.kube/config."
if [ -z "$PREBUILT" ] || [ "${1:-}" = "bundle" ]; then
  command -v git >/dev/null || die "git is required (sudo apt install git) — or run this script with the offline bundle, see the header"
  command -v curl >/dev/null || die "curl is required (sudo apt install curl)"
fi
mkdir -p "$BIN_DIR"

# ─── 1. Go toolchain (user-local, no sudo) ───────────────────────────────────
ensure_go() {
  if command -v go >/dev/null; then
    return
  fi
  if [ -x "$HOME/.local/go/bin/go" ]; then
    export PATH="$HOME/.local/go/bin:$PATH"
    return
  fi
  say "Installing Go ${GO_VERSION} to ~/.local/go"
  local arch
  case "$(uname -m)" in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m)" ;;
  esac
  local tgz="go${GO_VERSION}.linux-${arch}.tar.gz"
  curl -fsSL "https://go.dev/dl/${tgz}" -o "/tmp/${tgz}"
  rm -rf "$HOME/.local/go"
  tar -C "$HOME/.local" -xzf "/tmp/${tgz}"
  rm -f "/tmp/${tgz}"
  export PATH="$HOME/.local/go/bin:$PATH"
}

fetch_source() {
  if [ -d "$SRC_DIR/.git" ]; then
    git -C "$SRC_DIR" fetch -q origin "$REPO_REF"
    git -C "$SRC_DIR" checkout -q FETCH_HEAD
  else
    rm -rf "$SRC_DIR"
    git clone -q --depth 1 --branch "$REPO_REF" "$REPO_URL" "$SRC_DIR"
  fi
}

# ─── bundle: build a static binary + this script for hosts without internet ──
if [ "${1:-}" = "bundle" ]; then
  arch="${BUNDLE_ARCH:-amd64}"
  out="kubectl-sheep-offline-${arch}"
  say "Building offline bundle for linux/${arch}"
  ensure_go
  fetch_source
  rm -rf "$out" && mkdir -p "$out"
  (cd "$SRC_DIR" && GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
     go build -trimpath -ldflags="-s -w" -o "$OLDPWD/$out/kubectl-sheep" ./cmd/kubectl-sheep)
  cp "${BASH_SOURCE[0]}" "$out/wsl-setup.sh"
  if [ -n "${INSTANCES_FILE:-}" ] && [ -f "$INSTANCES_FILE" ]; then
    cp "$INSTANCES_FILE" "$out/instances.txt"
  fi
  tar -czf "$out.tar.gz" "$out"
  rm -rf "$out"
  say "Done: $out.tar.gz"
  note "Copy it to the host and run:  tar xzf $out.tar.gz && bash $out/wsl-setup.sh"
  note "(instances.txt inside the bundle is picked up automatically)"
  exit 0
fi

# ─── 2. Install kubectl-sheep ────────────────────────────────────────────────
if [ -n "$PREBUILT" ]; then
  say "Installing prebuilt kubectl-sheep from $PREBUILT"
  [ -x "$PREBUILT" ] || die "$PREBUILT is not an executable file"
  install -m 0755 "$PREBUILT" "$SHEEP"
  if [ -z "${INSTANCES_FILE:-}" ] && [ -f "$SCRIPT_DIR/instances.txt" ]; then
    mapfile -t INSTANCES < <(grep -Ev '^\s*(#|$)' "$SCRIPT_DIR/instances.txt")
    note "using instance list from $SCRIPT_DIR/instances.txt"
  fi
else
  say "Building kubectl-sheep from ${REPO_URL} (${REPO_REF})"
  ensure_go
  fetch_source
  (cd "$SRC_DIR" && GOTOOLCHAIN=auto go build -trimpath -o "$SHEEP" ./cmd/kubectl-sheep)
fi
note "installed $SHEEP ($("$SHEEP" version 2>/dev/null || echo dev))"

if ! command -v kubectl >/dev/null; then
  if [ "$IS_WSL" = 1 ]; then
    note "Note: kubectl is not installed in WSL. The Windows kubectl will still work; install it in WSL to use 'kubectl sheep …' here."
  else
    note "Note: kubectl is not installed; install it to use the fetched contexts on this host."
  fi
fi

# ─── 3. Share the Windows kubeconfig ─────────────────────────────────────────
say "Choosing the kubeconfig to write"
WIN_PROFILE=""
if [ "$IS_WSL" = 1 ] && command -v cmd.exe >/dev/null; then
  WIN_PROFILE="$(cmd.exe /C 'echo %USERPROFILE%' 2>/dev/null | tr -d '\r' || true)"
fi
if [ -n "$WIN_PROFILE" ] && command -v wslpath >/dev/null; then
  KUBECONFIG_PATH="$(wslpath "$WIN_PROFILE")/.kube/config"
else
  KUBECONFIG_PATH="$HOME/.kube/config"
  [ "$IS_WSL" = 1 ] && note "Windows profile not found; using $KUBECONFIG_PATH instead."
fi
mkdir -p "$(dirname "$KUBECONFIG_PATH")"
export KUBECONFIG="$KUBECONFIG_PATH"
note "KUBECONFIG=$KUBECONFIG_PATH"

# ─── 4. Register Rancher instances with AD login ─────────────────────────────
say "Registering Rancher instances"
note "You will be asked for your AD user name per instance and for the password once per directory."
DEFAULT_USER="${AD_USER:-${USER:-$(id -un)}}"
EXISTING="$("$SHEEP" rancher-instance list 2>/dev/null | awk 'NR>1 {print $1}' || true)"

for entry in "${INSTANCES[@]}"; do
  IFS='|' read -r name url ptype pid <<<"$entry"
  ptype="${ptype:-auto}"
  pid="${pid:-auto}"

  printf '\n  %s  (%s)\n' "$name" "$url"
  read -r -p "  AD user name [${DEFAULT_USER}]: " user
  user="${user:-$DEFAULT_USER}"
  DEFAULT_USER="$user"

  auth_flags=(--auth-login --auth-provider-type "$ptype" --auth-provider-id "$pid" --auth-username "$user")
  if grep -qx "$name" <<<"$EXISTING"; then
    "$SHEEP" rancher-instance update-token "$name" "${auth_flags[@]}" --no-input
  else
    "$SHEEP" rancher-instance add "$name" "$url" --storage "$STORAGE" "${auth_flags[@]}" --no-input
  fi

  "$SHEEP" kubeconfig get "$name" --all --no-input
done

# ─── 5. Auto-connect on shell start ──────────────────────────────────────────
say "Enabling auto-connect at shell start"
case "${SHELL:-}" in
  */zsh) RC="$HOME/.zshrc" ;;
  *)     RC="$HOME/.bashrc" ;;
esac
BLOCK_START="# >>> kubectl-sheep >>>"
BLOCK_END="# <<< kubectl-sheep <<<"
if grep -q "$BLOCK_START" "$RC" 2>/dev/null; then
  sed -i "/$BLOCK_START/,/$BLOCK_END/d" "$RC"
fi
cat >>"$RC" <<RC
$BLOCK_START
export PATH="\$HOME/.local/bin:\$PATH"
export KUBECONFIG="$KUBECONFIG_PATH"
# Validate Rancher tokens, renew expired ones (asks for the AD password), refresh kubeconfigs.
case \$- in *i*) command -v kubectl-sheep >/dev/null && kubectl-sheep connect --quiet ;; esac
$BLOCK_END
RC
note "added to $RC"

# ─── 6. First run ────────────────────────────────────────────────────────────
say "Running the first connect"
"$SHEEP" connect

say "Done"
note "Open a new shell (or log in again via SSH): tokens are checked automatically."
if [ -n "$WIN_PROFILE" ]; then
  note "Windows: kubectl / Lens / k9s use ${WIN_PROFILE}\\.kube\\config as usual — no plugin needed there."
fi
note "Contexts are named <instance>-<cluster>, e.g.:  kubectl config get-contexts"
