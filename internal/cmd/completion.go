package cmd

import (
	"context"
	"strings"

	"github.com/ahoz/kubectl-sheep/internal/config"
	"github.com/ahoz/kubectl-sheep/internal/instance"
	"github.com/ahoz/kubectl-sheep/internal/kubeconfig"
	"github.com/spf13/cobra"
)

func registerCompletions(root *cobra.Command) {
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			c.Short = "Generate shell autocompletion scripts"
			c.Long = `Generate autocompletion scripts for bash, zsh, fish, or PowerShell.

kubectl plugins are invoked as "kubectl sheep …"; load completion for the plugin:

  # bash
  source <(kubectl sheep completion bash)

  # zsh
  source <(kubectl sheep completion zsh)

🪄 After enabling completion, tab-complete rancher-instance names and cluster names/IDs
on commands that accept them.`
			c.Example = exCompletion
		}
	}

	registerRancherInstanceCompletions(root)
	registerKubeconfigCompletions(root)

	if connect := findSubCommand(root, "connect"); connect != nil {
		connect.ValidArgsFunction = completeRancherInstances
	}
}

func registerRancherInstanceCompletions(root *cobra.Command) {
	ri := findSubCommand(root, "rancher-instance")
	if ri == nil {
		return
	}

	for _, name := range []string{"remove", "update-token", "set-storage"} {
		if cmd := findSubCommand(ri, name); cmd != nil {
			cmd.ValidArgsFunction = completeRancherInstances
		}
	}

	if clusters := findSubCommand(ri, "clusters"); clusters != nil {
		if list := findSubCommand(clusters, "list"); list != nil {
			list.ValidArgsFunction = completeRancherInstances
		}
	}

	if add := findSubCommand(ri, "add"); add != nil {
		_ = add.RegisterFlagCompletionFunc("storage", completeStorageFlag)
	}
	if setStorage := findSubCommand(ri, "set-storage"); setStorage != nil {
		_ = setStorage.RegisterFlagCompletionFunc("to", completeStorageFlag)
	}
}

func registerKubeconfigCompletions(root *cobra.Command) {
	kc := findSubCommand(root, "kubeconfig")
	if kc == nil {
		return
	}

	for _, spec := range []struct {
		name    string
		argFunc func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	}{
		{"list", completeRancherInstances},
		{"get", completeKubeconfigGetArgs},
		{"refresh", completeKubeconfigRefreshArgs},
		{"install-exec", completeKubeconfigInstallExecArgs},
	} {
		if cmd := findSubCommand(kc, spec.name); cmd != nil {
			cmd.ValidArgsFunction = spec.argFunc
		}
	}
}

func findSubCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func completeRancherInstances(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names, err := listRancherInstanceNames()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions(names, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeStorageFlag(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	options := []string{config.StorageEncrypted, config.StoragePlaintext}
	return filterCompletions(options, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeKubeconfigGetArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeRancherInstances(nil, args, toComplete)
	case 1:
		return completeClusterRefs(args[0], true, toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeKubeconfigRefreshArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeRancherInstances(nil, args, toComplete)
	case 1:
		return completeClusterRefs(args[0], false, toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeKubeconfigInstallExecArgs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeRancherInstances(nil, args, toComplete)
	case 1:
		return completeClusterRefs(args[0], true, toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

func listRancherInstanceNames() ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(cfg.Instances))
	for i, inst := range cfg.Instances {
		names[i] = inst.Name
	}
	return names, nil
}

func completeClusterRefs(instanceName string, includeRemote bool, toComplete string) ([]string, cobra.ShellCompDirective) {
	refs, err := listClusterRefs(instanceName, includeRemote)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterCompletions(refs, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func listClusterRefs(instanceName string, includeRemote bool) ([]string, error) {
	seen := make(map[string]struct{})
	var refs []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		refs = append(refs, v)
	}

	ids, err := kubeconfig.ListStoredClusterIDs(instanceName)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		add(id)
		if meta, err := kubeconfig.LoadMetadata(instanceName, id); err == nil {
			add(meta.ClusterName)
		}
	}

	if !includeRemote {
		return refs, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return refs, nil
	}
	inst, err := cfg.Find(instanceName)
	if err != nil || inst.Storage == config.StorageEncrypted {
		return refs, nil
	}

	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return refs, nil
	}
	clusters, err := client.ListClusters(context.Background())
	if err != nil {
		return refs, nil
	}
	for _, c := range clusters {
		add(c.ID)
		add(c.Name)
	}
	return refs, nil
}

func filterCompletions(items []string, toComplete string) []string {
	if toComplete == "" {
		return items
	}
	var out []string
	for _, item := range items {
		if strings.HasPrefix(item, toComplete) {
			out = append(out, item)
		}
	}
	return out
}
