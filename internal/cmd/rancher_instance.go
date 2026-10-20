package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ahoz/kubectl-sheep/internal/browser"
	"github.com/ahoz/kubectl-sheep/internal/config"
	"github.com/ahoz/kubectl-sheep/internal/credentials"
	"github.com/ahoz/kubectl-sheep/internal/instance"
	"github.com/ahoz/kubectl-sheep/internal/prompt"
	"github.com/ahoz/kubectl-sheep/internal/rancher"
	"github.com/spf13/cobra"
)

func newRancherInstanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rancher-instance",
		Short:   "Manage Rancher instance connections",
		Long: `Register Rancher instances, manage API tokens, and inspect remote clusters.

🐑 Interactive commands (omit arguments on a TTY): add, remove, update-token,
clusters list.

🪄 Tab-complete instance names after: kubectl sheep completion bash`,
		Example: exRancherInstance,
	}

	cmd.AddCommand(newRancherInstanceAddCmd())
	cmd.AddCommand(newRancherInstanceListCmd())
	cmd.AddCommand(newRancherInstanceRemoveCmd())
	cmd.AddCommand(newRancherInstanceSetStorageCmd())
	cmd.AddCommand(newRancherInstanceUpdateTokenCmd())
	cmd.AddCommand(newRancherInstanceClustersCmd())

	return cmd
}

func newRancherInstanceClustersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clusters",
		Short:   "Inspect clusters on a Rancher instance",
		Long:    "List downstream clusters registered on a Rancher instance.",
		Example: exRancherInstanceClusters,
	}

	cmd.AddCommand(newRancherInstanceClustersListCmd())

	return cmd
}

func newRancherInstanceClustersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list [rancher-instance]",
		Short:   "List clusters on a Rancher instance",
		Long:    "Display all clusters registered on a Rancher instance. Omit the instance on a TTY to pick one interactively.",
		Example: exRancherInstanceClustersList,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractive(cmd) && len(args) == 0 {
				prompt.Intro(cmd.OutOrStdout(), "List clusters on Rancher instance")
			}

			instanceName, err := promptRancherInstance(cmd, args)
			if err != nil {
				return err
			}

			_, client, err := instance.RancherClient(instanceName)
			if err != nil {
				return err
			}

			clusters, err := client.ListClusters(context.Background())
			if err != nil {
				return handleRancherError(instanceName, err)
			}

			if len(clusters) == 0 {
				fprintln(cmd.OutOrStdout(), "No clusters found.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fprintln(w, "ID\tNAME\tSTATE")
			for _, c := range clusters {
				fprint(w, "%s\t%s\t%s\n", c.ID, c.Name, c.State)
			}
			return w.Flush()
		},
	}
}

func newRancherInstanceAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [name] [url]",
		Short:   "Add a Rancher instance",
		Long:    "Register a new Rancher instance with name, URL, and token storage preference. Run without arguments on a TTY for an interactive wizard.",
		Example: exRancherInstanceAdd,
		Args:    cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractive(cmd) && len(args) == 0 {
				prompt.Intro(cmd.OutOrStdout(), "Add Rancher instance")
			}

			name, url, err := resolveAddNameAndURL(cmd, args)
			if err != nil {
				return err
			}
			if err := config.ValidateURL(url); err != nil {
				return err
			}

			storage, _ := cmd.Flags().GetString("storage")
			insecure, _ := cmd.Flags().GetBool("insecure")
			openBrowser, _ := cmd.Flags().GetBool("open")
			authOpts, err := authLoginOptionsFromFlags(cmd)
			if err != nil {
				return err
			}

			storage, insecure, err = promptAddOptions(cmd, len(args) == 0, storage, insecure)
			if err != nil {
				return err
			}

			if !authOpts.enabled {
				openBrowser, err = promptOpenBrowser(cmd, openBrowser)
				if err != nil {
					return err
				}
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			inst := config.Instance{
				Name:               name,
				URL:                url,
				InsecureSkipVerify: insecure,
				Storage:            storage,
				Auth:               authOpts.toConfig(),
			}
			if err := cfg.AddInstance(inst); err != nil {
				return fmt.Errorf("add rancher-instance: %w", err)
			}

			if !authOpts.enabled {
				if tokenPageURL, err := rancher.TokenCreatePageURL(url); err != nil {
					return fmt.Errorf("build token page URL: %w", err)
				} else {
					if openBrowser {
						if err := browser.Open(tokenPageURL); err != nil {
							return err
						}
					}
					prompt.PrintTokenCreateHint(cmd.OutOrStdout(), tokenPageURL)
				}
			}

			token, err := readRancherToken(cmd, url, insecure, authOpts, "Rancher API token")
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("token must not be empty")
			}
			if authOpts.enabled {
				if stored, err := cfg.Find(name); err == nil {
					stored.Auth = lastResolvedAuth
				}
			}

			if storage == config.StorageEncrypted {
				credentials.SetNextPassphraseHint("You'll be prompted again whenever you fetch kubeconfigs.")
			}

			store, err := credentials.NewStore(storage)
			if err != nil {
				return err
			}
			if err := store.Set(name, token); err != nil {
				return err
			}

			if err := cfg.Save(); err != nil {
				_ = store.Delete(name)
				return err
			}

			fprint(cmd.OutOrStdout(), "Added rancher-instance %q\n", name)
			return nil
		},
	}

	cmd.Flags().String("storage", config.StorageEncrypted, "Token storage mode: plaintext or encrypted")
	cmd.Flags().Bool("insecure", false, "Skip TLS certificate verification")
	cmd.Flags().Bool("open", false, "Open the Rancher API key page in the default browser")
	addAuthLoginFlags(cmd)

	return cmd
}

func newRancherInstanceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List configured Rancher instances",
		Long:    "Display all registered Rancher instances and their settings.",
		Example: exRancherInstanceList,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if len(cfg.Instances) == 0 {
				fprintln(cmd.OutOrStdout(), "No rancher-instances configured.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fprintln(w, "NAME\tURL\tSTORAGE\tINSECURE\tAUTH LOGIN")
			for _, inst := range cfg.Instances {
				insecure := "false"
				if inst.InsecureSkipVerify {
					insecure = "true"
				}
				auth := "-"
				if inst.Auth != nil {
					auth = inst.Auth.String()
				}
				fprint(w, "%s\t%s\t%s\t%s\t%s\n", inst.Name, inst.URL, inst.Storage, insecure, auth)
			}
			return w.Flush()
		},
	}
}

func newRancherInstanceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove [name]",
		Short:   "Remove a Rancher instance",
		Long:    "Remove a Rancher instance and its stored credentials. Omit the name on a TTY to pick one interactively.",
		Example: exRancherInstanceRemove,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractive(cmd) && len(args) == 0 {
				prompt.Intro(cmd.OutOrStdout(), "Remove Rancher instance")
			}

			name, err := promptRancherInstance(cmd, args)
			if err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			inst, err := cfg.Find(name)
			if err != nil {
				return err
			}

			store, err := credentials.NewStore(inst.Storage)
			if err != nil {
				return err
			}
			_ = store.Delete(name)

			if err := cfg.RemoveInstance(name); err != nil {
				return fmt.Errorf("remove rancher-instance: %w", err)
			}
			if err := cfg.Save(); err != nil {
				return err
			}

			fprint(cmd.OutOrStdout(), "Removed rancher-instance %q\n", name)
			return nil
		},
	}
}

func newRancherInstanceSetStorageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-storage <name>",
		Short:   "Change token storage mode for an instance",
		Long:    "Migrate an instance's Rancher token between plaintext and encrypted storage.",
		Example: exRancherInstanceSetStorage,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			to, _ := cmd.Flags().GetString("to")

			if err := config.ValidateStorage(to); err != nil {
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			inst, err := cfg.Find(name)
			if err != nil {
				return err
			}

			if inst.Storage == to {
				return fmt.Errorf("rancher-instance %q already uses %q storage", name, to)
			}

			if err := credentials.MigrateStorage(name, inst.Storage, to); err != nil {
				return fmt.Errorf("migrate storage: %w", err)
			}

			inst.Storage = to
			if err := cfg.Save(); err != nil {
				return err
			}

			fprint(cmd.OutOrStdout(), "Migrated rancher-instance %q storage to %q\n", name, to)
			return nil
		},
	}

	cmd.Flags().String("to", "", "Target storage mode: plaintext or encrypted (required)")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

func newRancherInstanceUpdateTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update-token [name]",
		Short:   "Update the Rancher API token for an instance",
		Long:    "Set a new Rancher API token after the current one becomes invalid or expired. Omit the instance on a TTY to pick one interactively.",
		Example: exRancherInstanceUpdateToken,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractive(cmd) && len(args) == 0 {
				prompt.Intro(cmd.OutOrStdout(), "Update Rancher API token")
			}

			name, err := promptRancherInstance(cmd, args)
			if err != nil {
				return err
			}
			openBrowser, _ := cmd.Flags().GetBool("open")

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			inst, err := cfg.Find(name)
			if err != nil {
				return err
			}

			authOpts, err := resolveAuthLoginOptions(cmd, inst)
			if err != nil {
				return err
			}
			if authOpts.enabled && authOpts.fromConfig {
				prompt.Step(cmd.OutOrStdout())
				prompt.Note(cmd.OutOrStdout(), fmt.Sprintf("Using stored auth provider login: %s", inst.Auth))
				prompt.Note(cmd.OutOrStdout(), "Pass --auth-login=false to paste a Rancher API token instead.")
			}

			if !authOpts.enabled {
				if tokenPageURL, err := rancher.TokenCreatePageURL(inst.URL); err != nil {
					return fmt.Errorf("build token page URL: %w", err)
				} else {
					openBrowser, err = promptOpenBrowser(cmd, openBrowser)
					if err != nil {
						return err
					}
					if openBrowser {
						if err := browser.Open(tokenPageURL); err != nil {
							return err
						}
					}
					prompt.PrintTokenCreateHint(cmd.OutOrStdout(), tokenPageURL)
				}
			}

			token, err := readRancherToken(cmd, inst.URL, inst.InsecureSkipVerify, authOpts, "New Rancher API token")
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("token must not be empty")
			}

			client, err := rancher.NewClient(inst.URL, token, inst.InsecureSkipVerify)
			if err != nil {
				return err
			}
			if err := client.ValidateToken(context.Background()); err != nil {
				return handleRancherError(name, err)
			}

			store, err := credentials.NewStore(inst.Storage)
			if err != nil {
				return err
			}
			if err := store.Set(name, token); err != nil {
				return err
			}

			if authOpts.enabled && !authOpts.fromConfig {
				inst.Auth = lastResolvedAuth
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("save auth login settings: %w", err)
				}
			}

			fprint(cmd.OutOrStdout(), "Updated token for rancher-instance %q\n", name)
			return nil
		},
	}

	cmd.Flags().Bool("open", false, "Open the Rancher API key page in the default browser")
	addAuthLoginFlags(cmd)

	return cmd
}

type authLoginOptions struct {
	enabled      bool
	fromConfig   bool
	providerType string
	providerID   string
	username     string
}

// toConfig converts enabled options into persistable instance settings.
func (o authLoginOptions) toConfig() *config.AuthLogin {
	if !o.enabled {
		return nil
	}
	return &config.AuthLogin{
		ProviderType: o.providerType,
		ProviderID:   o.providerID,
		Username:     o.username,
	}
}

func authLoginOptionsFromConfig(auth *config.AuthLogin) authLoginOptions {
	if auth == nil {
		return authLoginOptions{}
	}
	return authLoginOptions{
		enabled:      true,
		fromConfig:   true,
		providerType: auth.ProviderType,
		providerID:   auth.ProviderID,
		username:     auth.Username,
	}
}

func addAuthLoginFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("auth-login", false, "Create a Rancher API token by logging in through an auth provider")
	cmd.Flags().String("auth-provider-type", "activeDirectory", "Rancher auth provider type for --auth-login (\"auto\" detects it from the instance)")
	cmd.Flags().String("auth-provider-id", "activeDirectory", "Rancher auth provider ID for --auth-login (\"auto\" detects it from the instance)")
	cmd.Flags().String("auth-username", "", "Username for --auth-login")
	cmd.Flags().Bool("ldap-login", false, "Shortcut for --auth-login --auth-provider-type=ldap --auth-provider-id=openldap")
	cmd.Flags().String("ldap-provider", "openldap", "Rancher LDAP provider ID for --ldap-login")
	cmd.Flags().String("ldap-username", "", "LDAP username for --ldap-login")
}

func authLoginOptionsFromFlags(cmd *cobra.Command) (authLoginOptions, error) {
	authLogin, _ := cmd.Flags().GetBool("auth-login")
	ldapLogin, _ := cmd.Flags().GetBool("ldap-login")
	if !authLogin && !ldapLogin {
		return authLoginOptions{}, nil
	}

	if authLogin && ldapLogin {
		return authLoginOptions{}, fmt.Errorf("use either --auth-login or --ldap-login, not both")
	}

	if ldapLogin {
		ldapProvider, _ := cmd.Flags().GetString("ldap-provider")
		ldapUsername, _ := cmd.Flags().GetString("ldap-username")
		return authLoginOptions{
			enabled:      true,
			providerType: "ldap",
			providerID:   ldapProvider,
			username:     ldapUsername,
		}, nil
	}

	providerType, _ := cmd.Flags().GetString("auth-provider-type")
	providerID, _ := cmd.Flags().GetString("auth-provider-id")
	username, _ := cmd.Flags().GetString("auth-username")
	return authLoginOptions{
		enabled:      true,
		providerType: providerType,
		providerID:   providerID,
		username:     username,
	}, nil
}

// resolveAuthLoginOptions combines command-line flags with the auth login
// stored for an instance. Flags win; without any auth flag the stored login is
// used; --auth-login=false forces the manual token prompt.
func resolveAuthLoginOptions(cmd *cobra.Command, inst *config.Instance) (authLoginOptions, error) {
	opts, err := authLoginOptionsFromFlags(cmd)
	if err != nil {
		return authLoginOptions{}, err
	}
	if opts.enabled {
		if opts.username == "" && inst != nil && inst.Auth != nil {
			opts.username = inst.Auth.Username
		}
		return opts, nil
	}

	explicitlyDisabled := cmd.Flags().Changed("auth-login") || cmd.Flags().Changed("ldap-login")
	if explicitlyDisabled || inst == nil || inst.Auth == nil {
		return authLoginOptions{}, nil
	}
	return authLoginOptionsFromConfig(inst.Auth), nil
}

func readRancherToken(cmd *cobra.Command, rancherURL string, insecure bool, authOpts authLoginOptions, tokenLabel string) (string, error) {
	if !authOpts.enabled {
		return prompt.ReadSecret(os.Stdin, cmd.OutOrStdout(), tokenLabel)
	}

	if authOpts.username == "" {
		return "", fmt.Errorf("--auth-username is required with --auth-login")
	}

	auth, err := resolveAutoAuthProvider(context.Background(), rancherURL, insecure, authOpts.toConfig())
	if err != nil {
		return "", err
	}
	lastResolvedAuth = auth
	if auth.ProviderType != authOpts.providerType || auth.ProviderID != authOpts.providerID {
		prompt.Note(cmd.OutOrStdout(), fmt.Sprintf("Detected auth provider: %s", auth))
	}

	password, err := prompt.ReadSecret(os.Stdin, cmd.OutOrStdout(), fmt.Sprintf("Auth provider password for %s", auth.Username))
	if err != nil {
		return "", err
	}

	return createTokenViaAuthLogin(context.Background(), rancherURL, insecure, auth, password)
}

// lastResolvedAuth holds the auth login readRancherToken actually used, with
// "auto" provider settings replaced by the detected provider, so that add and
// update-token persist concrete values.
var lastResolvedAuth *config.AuthLogin

// autoAuthProvider is the flag value that asks kubectl-sheep to detect the
// provider from the instance's public auth provider list.
const autoAuthProvider = "auto"

// resolveAutoAuthProvider replaces "auto" provider settings with the one
// password-capable provider enabled on the instance. A directory provider is
// preferred over Rancher's local accounts; several directory providers are an
// error that lists them so the user can pick one explicitly.
func resolveAutoAuthProvider(ctx context.Context, rancherURL string, insecure bool, auth *config.AuthLogin) (*config.AuthLogin, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth login must not be nil")
	}
	if auth.ProviderType != autoAuthProvider && auth.ProviderID != autoAuthProvider {
		return auth, nil
	}

	client, err := rancher.NewClient(rancherURL, "", insecure)
	if err != nil {
		return nil, err
	}
	providers, err := client.ListAuthProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect auth provider: %w", err)
	}

	candidates := rancher.PasswordLoginProviders(providers)
	var directory, local []rancher.AuthProvider
	for _, p := range candidates {
		if p.Type == "localProvider" {
			local = append(local, p)
		} else {
			directory = append(directory, p)
		}
	}

	var chosen rancher.AuthProvider
	switch {
	case len(directory) == 1:
		chosen = directory[0]
	case len(directory) == 0 && len(local) == 1:
		chosen = local[0]
	case len(directory) == 0:
		return nil, fmt.Errorf("no auth provider with password login is enabled on %s (enabled: %s); create a Rancher API token instead", rancherURL, describeProviders(providers))
	default:
		return nil, fmt.Errorf("several directory providers are enabled on %s (%s); pass --auth-provider-type and --auth-provider-id explicitly", rancherURL, describeProviders(directory))
	}

	resolved := *auth
	resolved.ProviderType = chosen.LoginType()
	resolved.ProviderID = chosen.ID
	return &resolved, nil
}

func describeProviders(providers []rancher.AuthProvider) string {
	if len(providers) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(providers))
	for _, p := range providers {
		parts = append(parts, fmt.Sprintf("--auth-provider-type %s --auth-provider-id %s", p.LoginType(), p.ID))
	}
	return strings.Join(parts, "; ")
}

// authLoginHint appends the list of usable providers to an error that is not a
// plain wrong-password failure, e.g. Rancher's 500 "authconfigs ... not found"
// when the configured provider does not exist on the instance.
func authLoginHint(ctx context.Context, rancherURL string, insecure bool, err error) error {
	if err == nil || errors.Is(err, rancher.ErrLoginFailed) {
		return err
	}
	client, cerr := rancher.NewClient(rancherURL, "", insecure)
	if cerr != nil {
		return err
	}
	providers, lerr := client.ListAuthProviders(ctx)
	if lerr != nil {
		return err
	}
	return fmt.Errorf("%w\n  providers with password login on %s: %s", err, rancherURL, describeProviders(rancher.PasswordLoginProviders(providers)))
}

// createTokenViaAuthLogin logs in through a Rancher auth provider and converts
// the resulting session into a long-lived Rancher API token.
func createTokenViaAuthLogin(ctx context.Context, rancherURL string, insecure bool, auth *config.AuthLogin, password string) (string, error) {
	auth, err := resolveAutoAuthProvider(ctx, rancherURL, insecure, auth)
	if err != nil {
		return "", err
	}
	if err := auth.Validate(); err != nil {
		return "", err
	}

	client, err := rancher.NewClient(rancherURL, "", insecure)
	if err != nil {
		return "", err
	}

	sessionToken, err := client.LoginAuthProvider(ctx, auth.ProviderType, auth.ProviderID, auth.Username, password)
	if err != nil {
		return "", authLoginHint(ctx, rancherURL, insecure, err)
	}

	tokenClient, err := rancher.NewClient(rancherURL, sessionToken, insecure)
	if err != nil {
		return "", err
	}
	token, err := tokenClient.CreateAPIToken(ctx, "kubectl-sheep")
	if err != nil {
		return "", err
	}
	return token, nil
}
