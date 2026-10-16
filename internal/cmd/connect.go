package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ahoz/kubectl-sheep/internal/config"
	"github.com/ahoz/kubectl-sheep/internal/credentials"
	"github.com/ahoz/kubectl-sheep/internal/kubeconfig"
	"github.com/ahoz/kubectl-sheep/internal/prompt"
	"github.com/ahoz/kubectl-sheep/internal/rancher"
	"github.com/spf13/cobra"
)

// maxPasswordAttempts bounds how often connect asks for an auth provider password.
const maxPasswordAttempts = 3

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect [rancher-instance...]",
		Short: "Validate Rancher tokens, renew expired ones, and refresh stored kubeconfigs",
		Long: `Check every configured Rancher instance (or only the given ones), renew
expired API tokens, and refresh the kubeconfigs stored under ~/.kube/sheep/.

Designed to run at shell startup, for example from ~/.bashrc in WSL:

  command -v kubectl-sheep >/dev/null && kubectl sheep connect --quiet

When Rancher rejects a token and the instance was registered with an auth
provider login (--auth-login / --ldap-login), connect asks once for that
provider password and renews the tokens of all instances sharing the same
login. Instances without a stored auth login are reported and must be renewed
with: kubectl sheep rancher-instance update-token <instance>

Contexts installed with "kubeconfig install-exec" load credentials on demand and
are left untouched.`,
		Example: exConnect,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnect(cmd, args)
		},
	}

	cmd.Flags().Bool("no-refresh", false, "Only validate and renew tokens; do not refresh stored kubeconfigs")
	cmd.Flags().BoolP("quiet", "q", false, "Only print prompts, renewals, and errors")

	return cmd
}

func runConnect(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Instances) == 0 {
		fprintln(out, "No rancher-instances configured; run: kubectl sheep rancher-instance add")
		return nil
	}

	instances, err := selectConnectInstances(cfg, args)
	if err != nil {
		return err
	}

	quiet, _ := cmd.Flags().GetBool("quiet")
	noRefresh, _ := cmd.Flags().GetBool("no-refresh")

	session := newConnectSession(out, quiet, noRefresh)
	if isInteractive(cmd) {
		session.readPassword = func(label, hint string) (string, error) {
			return prompt.ReadSecret(os.Stdin, out, label, hint)
		}
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	failures := 0
	for i := range instances {
		if err := session.connectInstance(ctx, &instances[i]); err != nil {
			failures++
			fprint(out, "✗ %s: %v\n", instances[i].Name, err)
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d rancher-instances failed", failures, len(instances))
	}
	return nil
}

// selectConnectInstances returns all instances when no names are given,
// otherwise the named instances in the given order.
func selectConnectInstances(cfg *config.Config, names []string) ([]config.Instance, error) {
	if len(names) == 0 {
		return append([]config.Instance(nil), cfg.Instances...), nil
	}

	seen := make(map[string]struct{}, len(names))
	selected := make([]config.Instance, 0, len(names))
	for _, name := range names {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		inst, err := cfg.Find(name)
		if err != nil {
			return nil, err
		}
		selected = append(selected, *inst)
	}
	return selected, nil
}

// passwordReader prompts for an auth provider password. hint is shown above the prompt.
type passwordReader func(label, hint string) (string, error)

type connectSession struct {
	out          io.Writer
	readPassword passwordReader // nil when prompts are not possible
	quiet        bool
	noRefresh    bool
	passwords    map[string][]string // auth login key → passwords that worked, most recent first
}

func newConnectSession(out io.Writer, quiet, noRefresh bool) *connectSession {
	return &connectSession{
		out:       out,
		quiet:     quiet,
		noRefresh: noRefresh,
		passwords: make(map[string][]string),
	}
}

// rememberPassword stores a working password for a login, most recent first.
func (s *connectSession) rememberPassword(key, password string) {
	known := s.passwords[key]
	for i, p := range known {
		if p == password {
			known = append(known[:i], known[i+1:]...)
			break
		}
	}
	s.passwords[key] = append([]string{password}, known...)
}

// printf always writes; info writes only when not in quiet mode.
func (s *connectSession) printf(format string, a ...any) {
	fprint(s.out, format, a...)
}

func (s *connectSession) info(format string, a ...any) {
	if s.quiet {
		return
	}
	fprint(s.out, format, a...)
}

func (s *connectSession) connectInstance(ctx context.Context, inst *config.Instance) error {
	store, err := credentials.NewStore(inst.Storage)
	if err != nil {
		return err
	}

	var (
		client     *rancher.Client
		needsToken string // empty when the stored token works
	)

	token, err := store.Get(inst.Name)
	switch {
	case err == nil:
		client, err = rancher.NewClient(inst.URL, token, inst.InsecureSkipVerify)
		if err != nil {
			return err
		}
		switch err := client.ValidateToken(ctx); {
		case err == nil:
		case errors.Is(err, rancher.ErrTokenInvalid):
			needsToken = "token invalid or expired"
		default:
			return err
		}
	case errors.Is(err, credentials.ErrWrongPassphrase):
		return err
	default:
		needsToken = "no token stored"
	}

	renewed := false
	if needsToken != "" {
		token, err := s.renewToken(ctx, inst, needsToken)
		if err != nil {
			return err
		}
		client, err = rancher.NewClient(inst.URL, token, inst.InsecureSkipVerify)
		if err != nil {
			return err
		}
		if err := client.ValidateToken(ctx); err != nil {
			return fmt.Errorf("renewed token rejected by rancher: %w", err)
		}
		if err := store.Set(inst.Name, token); err != nil {
			return err
		}
		renewed = true
		s.printf("✓ %s: token renewed via %s\n", inst.Name, inst.Auth)
	}

	if s.noRefresh {
		if !renewed {
			s.info("✓ %s: token valid\n", inst.Name)
		}
		return nil
	}

	return s.refreshStored(ctx, inst, client, renewed)
}

// renewToken obtains a new Rancher API token through the instance's stored
// auth provider login, asking for the password at most maxPasswordAttempts times.
func (s *connectSession) renewToken(ctx context.Context, inst *config.Instance, reason string) (string, error) {
	if inst.Auth == nil {
		return "", fmt.Errorf("%s; run: kubectl sheep rancher-instance update-token %s", reason, inst.Name)
	}
	if err := inst.Auth.Validate(); err != nil {
		return "", fmt.Errorf("%s; stored auth login is incomplete (%v); run: kubectl sheep rancher-instance update-token %s --auth-login --auth-username <user>", reason, err, inst.Name)
	}
	if s.readPassword == nil {
		return "", fmt.Errorf("%s; run \"kubectl sheep connect %s\" on a terminal to log in as %s, or: kubectl sheep rancher-instance update-token %s", reason, inst.Name, inst.Auth.Username, inst.Name)
	}

	key := inst.Auth.Key()

	// Several instances usually share one directory, so try every password
	// that already worked for this login before asking again. An instance
	// bound to a different directory (other password) falls through to the prompt.
	for _, password := range s.passwords[key] {
		token, err := createTokenViaAuthLogin(ctx, inst.URL, inst.InsecureSkipVerify, inst.Auth, password)
		if err == nil {
			s.rememberPassword(key, password)
			return token, nil
		}
		if !errors.Is(err, rancher.ErrLoginFailed) {
			return "", err
		}
	}
	if len(s.passwords[key]) > 0 {
		s.printf("  %s: the password used so far is not accepted for %s (different directory?)\n", inst.Name, inst.Auth)
	}

	for attempt := 1; attempt <= maxPasswordAttempts; attempt++ {
		hint := fmt.Sprintf("Rancher-instance %q: %s. Log in via %s to renew it.", inst.Name, reason, inst.Auth)
		password, err := s.readPassword(fmt.Sprintf("Password for %s @ %s", inst.Auth.Username, inst.Name), hint)
		if err != nil {
			return "", err
		}
		if password == "" {
			return "", fmt.Errorf("no password entered; token not renewed")
		}

		token, err := createTokenViaAuthLogin(ctx, inst.URL, inst.InsecureSkipVerify, inst.Auth, password)
		if err == nil {
			s.rememberPassword(key, password)
			return token, nil
		}
		if !errors.Is(err, rancher.ErrLoginFailed) {
			return "", err
		}
		s.printf("  %s: login as %s failed: invalid username or password\n", inst.Name, inst.Auth.Username)
	}
	return "", fmt.Errorf("auth provider login failed after %d attempts", maxPasswordAttempts)
}

// refreshStored re-fetches every locally stored kubeconfig for the instance,
// except clusters whose merged context is exec-based (they load on demand).
func (s *connectSession) refreshStored(ctx context.Context, inst *config.Instance, client *rancher.Client, renewed bool) error {
	report := s.info
	if renewed {
		report = s.printf
	}

	storedIDs, err := kubeconfig.ListStoredClusterIDs(inst.Name)
	if err != nil {
		return err
	}
	if len(storedIDs) == 0 {
		report("✓ %s: connected, no stored kubeconfigs (fetch with: kubectl sheep kubeconfig get %s --all)\n", inst.Name, inst.Name)
		return nil
	}

	toRefresh, missing, err := storedClustersToRefresh(ctx, client, storedIDs)
	if err != nil {
		return handleRancherError(inst.Name, err)
	}
	for _, id := range missing {
		s.info("  SKIP %s: cluster no longer exists in Rancher\n", id)
	}

	toRefresh, execCount, err := withoutExecContexts(inst.Name, toRefresh)
	if err != nil {
		return err
	}

	results := refreshClusters(ctx, inst.Name, client, toRefresh)
	updated, failures := 0, 0
	for _, r := range results {
		if r.err != nil {
			failures++
			s.printf("  ERROR %s (%s): %v\n", r.cluster.Name, r.cluster.ID, r.err)
			continue
		}
		if r.changed {
			updated++
		}
	}

	summary := fmt.Sprintf("%s refreshed, %d updated", plural(len(results)-failures, "kubeconfig"), updated)
	if execCount > 0 {
		summary += fmt.Sprintf(", %s on demand", plural(execCount, "exec context"))
	}
	if failures > 0 {
		for _, r := range results {
			if errors.Is(r.err, rancher.ErrTokenInvalid) {
				return handleRancherError(inst.Name, rancher.ErrTokenInvalid)
			}
		}
		return fmt.Errorf("%s, %d failed", summary, failures)
	}

	report("✓ %s: connected, %s\n", inst.Name, summary)
	return nil
}

// withoutExecContexts drops clusters whose merged kubectl context uses an exec
// credential plugin, so a refresh does not replace it with an embedded token.
func withoutExecContexts(instanceName string, clusters []rancher.Cluster) ([]rancher.Cluster, int, error) {
	configPath, err := kubeconfigPath()
	if err != nil {
		return nil, 0, err
	}
	dest, err := loadKubeConfig(configPath)
	if err != nil {
		return nil, 0, err
	}

	kept := make([]rancher.Cluster, 0, len(clusters))
	execCount := 0
	for _, c := range clusters {
		ctxName := mergeContextName(instanceName, c.Name)
		if kctx, ok := dest.Contexts[ctxName]; ok {
			if auth, ok := dest.AuthInfos[kctx.AuthInfo]; ok && auth != nil && auth.Exec != nil {
				execCount++
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept, execCount, nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
