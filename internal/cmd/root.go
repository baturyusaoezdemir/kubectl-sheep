package cmd

import (
	"github.com/spf13/cobra"
)

const (
	shortDescription = "Fetch and manage kubeconfigs from Rancher-managed clusters"
	longDescription  = `A kubectl plugin to manage multiple Rancher instances, list their
downstream clusters, and fetch or refresh kubeconfigs individually or in bulk.

Kubeconfigs are cached under ~/.kube/sheep/ and merged into ~/.kube/config as
<rancher-instance>-<cluster-name> contexts.

Rancher API tokens can be stored as plaintext or encrypted (passphrase-protected),
selectable per instance.

On a TTY, many commands prompt for missing arguments with an interactive menu (🐑).
Use --no-input to disable prompts for scripts and CI.

Run "kubectl sheep connect" at shell startup to validate tokens, renew expired
ones through a Rancher auth provider login, and refresh stored kubeconfigs.

🪄 Enable shell completion with: kubectl sheep completion bash`
)

// NewRootCmd returns the root command for kubectl sheep.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "sheep",
		Short:        shortDescription,
		Long:         longDescription,
		Example:      exRoot,
		SilenceUsage: true,
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newConnectCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newRancherInstanceCmd())
	root.AddCommand(newKubeconfigCmd())

	root.PersistentFlags().Bool("no-input", false, "Disable interactive prompts")

	root.InitDefaultCompletionCmd()
	registerCompletions(root)

	return root
}
