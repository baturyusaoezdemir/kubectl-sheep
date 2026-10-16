package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ahoz/kubectl-sheep/internal/instance"
	"github.com/ahoz/kubectl-sheep/internal/kubeconfig"
	"github.com/ahoz/kubectl-sheep/internal/prompt"
	"github.com/ahoz/kubectl-sheep/internal/rancher"
	"github.com/spf13/cobra"
)

func newKubeconfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "kubeconfig",
		Short:   "Manage locally stored kubeconfigs",
		Long: `Download, refresh, list, and install kubeconfigs from Rancher-managed clusters.

Fetched kubeconfigs are saved under ~/.kube/sheep/ and automatically merged into
~/.kube/config with context names <rancher-instance>-<cluster-name>.

🐑 Interactive commands (omit arguments on a TTY): list, get, refresh.

🪄 Tab-complete instance and cluster arguments after: kubectl sheep completion bash`,
		Example: exKubeconfig,
	}

	cmd.AddCommand(newKubeconfigListCmd())
	cmd.AddCommand(newKubeconfigGetCmd())
	cmd.AddCommand(newKubeconfigRefreshCmd())
	cmd.AddCommand(newKubeconfigInstallExecCmd())

	return cmd
}

func newKubeconfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list [rancher-instance]",
		Short:   "List locally stored kubeconfigs",
		Long:    "Display kubeconfigs already downloaded for a Rancher instance. Omit the instance on a TTY to pick one interactively.",
		Example: exKubeconfigList,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractive(cmd) && len(args) == 0 {
				prompt.Intro(cmd.OutOrStdout(), "List locally stored kubeconfigs")
			}

			instanceName, err := promptRancherInstance(cmd, args)
			if err != nil {
				return err
			}

			ids, err := kubeconfig.ListStoredClusterIDs(instanceName)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				fprintln(cmd.OutOrStdout(), "No locally stored kubeconfigs.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fprintln(w, "CLUSTER ID\tCLUSTER NAME\tFETCHED AT\tPATH")
			for _, id := range ids {
				meta, err := kubeconfig.LoadMetadata(instanceName, id)
				if err != nil {
					fprint(w, "%s\t-\t-\t-\n", id)
					continue
				}
				path, err := kubeconfig.KubeconfigPath(instanceName, id)
				if err != nil {
					path = "-"
				}
				fprint(w, "%s\t%s\t%s\t%s\n", id, meta.ClusterName, meta.FetchedAt.Format("2006-01-02 15:04:05"), path)
			}
			return w.Flush()
		},
	}
}

func newKubeconfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get [rancher-instance] [cluster]",
		Short:   "Fetch kubeconfigs from Rancher",
		Long:    "Download, store, and merge kubeconfigs for one or more clusters, or for all clusters with --all. Omit arguments on a TTY to pick an instance and scope interactively.",
		Example: exKubeconfigGet,
		Args:    cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKubeconfigGet(cmd, args)
		},
	}

	cmd.Flags().Bool("all", false, "Fetch kubeconfigs for all clusters on the Rancher instance")

	return cmd
}

func runKubeconfigGet(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")

	if all && len(args) == 2 {
		return fmt.Errorf("use either a cluster argument or --all, not both")
	}

	if isInteractive(cmd) && len(args) < 2 {
		prompt.Intro(cmd.OutOrStdout(), "Fetch kubeconfigs from Rancher")
	}

	instanceName, err := promptRancherInstance(cmd, args)
	if err != nil {
		return err
	}

	if len(args) == 2 {
		return fetchSingleCluster(cmd, instanceName, args[1], false)
	}

	if all {
		return runFetchAll(cmd, instanceName)
	}

	if len(args) == 1 && !isInteractive(cmd) {
		return fmt.Errorf("specify a cluster or pass --all")
	}

	if isInteractive(cmd) {
		scope, err := promptGetScope(cmd)
		if err != nil {
			return err
		}
		switch scope {
		case getScopeAll:
			return runFetchAll(cmd, instanceName)
		case getScopeMultiple:
			clusters, err := promptClustersMulti(cmd, instanceName)
			if err != nil {
				return err
			}
			policy, err := resolveOverwritePolicy(cmd, instanceName, clusters)
			if err != nil {
				return err
			}
			clusters, err = filterClustersForFetch(cmd, instanceName, clusters, policy)
			if err != nil {
				return err
			}
			if len(clusters) == 0 {
				fprintln(cmd.OutOrStdout(), "No kubeconfigs to fetch.")
				return nil
			}
			return runFetchSelected(cmd, instanceName, clusters)
		default:
			clusterRef, err := promptCluster(cmd, instanceName)
			if err != nil {
				return err
			}
			return fetchSingleCluster(cmd, instanceName, clusterRef, false)
		}
	}

	return fmt.Errorf("specify a cluster or pass --all")
}

func newKubeconfigRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "refresh [rancher-instance] [cluster]",
		Short:   "Refresh locally stored kubeconfigs",
		Long:    "Re-fetch kubeconfigs for one cluster or for all locally stored clusters with --all. Omit arguments on a TTY to pick an instance and scope interactively.",
		Example: exKubeconfigRefresh,
		Args:    cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKubeconfigRefresh(cmd, args)
		},
	}

	cmd.Flags().Bool("all", false, "Refresh all locally stored kubeconfigs for the Rancher instance")

	return cmd
}

func runKubeconfigRefresh(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")

	if all && len(args) == 2 {
		return fmt.Errorf("use either a cluster argument or --all, not both")
	}

	if isInteractive(cmd) && len(args) < 2 && !all {
		prompt.Intro(cmd.OutOrStdout(), "Refresh locally stored kubeconfigs")
	}

	instanceName, err := promptRancherInstance(cmd, args)
	if err != nil {
		return err
	}

	if len(args) == 2 {
		clusterRef := args[1]
		exists, err := localClusterExists(instanceName, clusterRef)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("no local kubeconfig found for cluster %q; use kubeconfig get first", clusterRef)
		}
		return fetchSingleCluster(cmd, instanceName, clusterRef, true)
	}

	if all {
		return runRefreshAll(cmd, instanceName)
	}

	if len(args) == 1 && !isInteractive(cmd) {
		return fmt.Errorf("specify a cluster or pass --all")
	}

	if isInteractive(cmd) {
		scopeAll, err := promptRefreshScope(cmd)
		if err != nil {
			return err
		}
		if scopeAll {
			return runRefreshAll(cmd, instanceName)
		}
		clusterRef, err := promptStoredCluster(cmd, instanceName)
		if err != nil {
			return err
		}
		return fetchSingleCluster(cmd, instanceName, clusterRef, true)
	}

	return fmt.Errorf("specify a cluster or pass --all")
}

func newKubeconfigInstallExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install-exec <rancher-instance> <cluster>",
		Short:   "Install an exec-based kubeconfig context",
		Long:    "Create or update a kubeconfig context that calls kubectl-sheep to load credentials on demand.",
		Example: exKubeconfigInstallExec,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			execCommand, _ := cmd.Flags().GetString("exec-command")
			return installExecCluster(cmd, args[0], args[1], execCommand)
		},
	}

	cmd.Flags().String("exec-command", "kubectl-sheep", "Command used by kubeconfig exec users")

	return cmd
}

func runFetchAll(cmd *cobra.Command, instanceName string) error {
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

	policy, err := resolveOverwritePolicy(cmd, instanceName, clusters)
	if err != nil {
		return err
	}

	clusters, err = filterClustersForFetch(cmd, instanceName, clusters, policy)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		fprintln(cmd.OutOrStdout(), "No kubeconfigs to fetch.")
		return nil
	}

	return runFetchSelected(cmd, instanceName, clusters)
}

func runFetchSelected(cmd *cobra.Command, instanceName string, clusters []rancher.Cluster) error {
	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return err
	}

	results := fetchClusters(context.Background(), instanceName, client, clusters)
	failures := printFetchResults(cmd.OutOrStdout(), instanceName, results, isInteractive(cmd))
	if failures > 0 {
		for _, r := range results {
			if errors.Is(r.err, rancher.ErrTokenInvalid) {
				return handleRancherError(instanceName, rancher.ErrTokenInvalid)
			}
		}
		return fmt.Errorf("%d of %d clusters failed", failures, len(clusters))
	}
	return nil
}

func runRefreshAll(cmd *cobra.Command, instanceName string) error {
	storedIDs, err := kubeconfig.ListStoredClusterIDs(instanceName)
	if err != nil {
		return err
	}
	if len(storedIDs) == 0 {
		fprintln(cmd.OutOrStdout(), "No locally stored kubeconfigs.")
		return nil
	}

	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return err
	}

	toRefresh, missing, err := storedClustersToRefresh(context.Background(), client, storedIDs)
	if err != nil {
		return handleRancherError(instanceName, err)
	}
	for _, id := range missing {
		fprint(cmd.OutOrStdout(), "SKIP %s: cluster no longer exists in Rancher\n", id)
	}

	if len(toRefresh) == 0 {
		return nil
	}

	results := refreshClusters(context.Background(), instanceName, client, toRefresh)
	failures := printRefreshResults(cmd.OutOrStdout(), instanceName, results, isInteractive(cmd))
	if failures > 0 {
		for _, r := range results {
			if errors.Is(r.err, rancher.ErrTokenInvalid) {
				return handleRancherError(instanceName, rancher.ErrTokenInvalid)
			}
		}
		return fmt.Errorf("%d of %d clusters failed", failures, len(toRefresh))
	}
	return nil
}

// storedClustersToRefresh matches locally stored cluster IDs against the
// clusters currently known to Rancher. IDs that no longer exist are returned
// separately so callers can report them.
func storedClustersToRefresh(ctx context.Context, client clusterLister, storedIDs []string) (toRefresh []rancher.Cluster, missing []string, err error) {
	allClusters, err := client.ListClusters(ctx)
	if err != nil {
		return nil, nil, err
	}

	byID := make(map[string]rancher.Cluster, len(allClusters))
	for _, c := range allClusters {
		byID[c.ID] = c
	}

	for _, id := range storedIDs {
		c, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		toRefresh = append(toRefresh, c)
	}
	return toRefresh, missing, nil
}

type clusterLister interface {
	ListClusters(context.Context) ([]rancher.Cluster, error)
}

func installExecCluster(cmd *cobra.Command, instanceName, clusterRef, execCommand string) error {
	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return err
	}

	clusters, err := client.ListClusters(context.Background())
	if err != nil {
		return handleRancherError(instanceName, err)
	}

	cluster, err := rancher.FindCluster(clusters, clusterRef)
	if err != nil {
		return err
	}

	content, err := client.GenerateKubeconfig(context.Background(), cluster.ID)
	if err != nil {
		return handleRancherError(instanceName, err)
	}

	if err := kubeconfig.Save(instanceName, cluster.ID, cluster.Name, content); err != nil {
		return err
	}

	contextName := mergeContextName(instanceName, cluster.Name)
	execContent, err := buildExecKubeconfig(content, execKubeconfigOptions{
		contextName: contextName,
		command:     execCommand,
		args:        []string{"auth", "exec", instanceName, cluster.ID},
	})
	if err != nil {
		return err
	}

	if err := mergeKubeconfig(instanceName, cluster.Name, execContent); err != nil {
		return err
	}

	path, err := kubeconfig.KubeconfigPath(instanceName, cluster.ID)
	if err != nil {
		return err
	}
	configPath, err := kubeconfigPath()
	if err != nil {
		return err
	}
	reportKubeconfigSaved(cmd.OutOrStdout(), isInteractive(cmd), cluster.Name, path, contextName, configPath)
	return nil
}

func localClusterExists(instanceName, clusterRef string) (bool, error) {
	ids, err := kubeconfig.ListStoredClusterIDs(instanceName)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == clusterRef {
			return true, nil
		}
	}

	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return false, err
	}
	clusters, err := client.ListClusters(context.Background())
	if err != nil {
		return false, handleRancherError(instanceName, err)
	}
	cluster, err := rancher.FindCluster(clusters, clusterRef)
	if err != nil {
		return false, err
	}
	return kubeconfig.Exists(instanceName, cluster.ID)
}

func fetchSingleCluster(cmd *cobra.Command, instanceName, clusterRef string, refresh bool) error {
	_, client, err := instance.RancherClient(instanceName)
	if err != nil {
		return err
	}

	clusters, err := client.ListClusters(context.Background())
	if err != nil {
		return handleRancherError(instanceName, err)
	}

	cluster, err := rancher.FindCluster(clusters, clusterRef)
	if err != nil {
		return err
	}

	if !refresh {
		policy := overwriteAll
		if isInteractive(cmd) {
			var err error
			policy, err = resolveOverwritePolicy(cmd, instanceName, []rancher.Cluster{*cluster})
			if err != nil {
				return err
			}
		}
		ok, err := shouldOverwriteSingle(cmd, instanceName, *cluster, policy)
		if err != nil {
			return err
		}
		if !ok {
			fprint(cmd.OutOrStdout(), "Skipped %q: keeping existing kubeconfig\n", cluster.Name)
			return nil
		}
	}

	var previous string
	if refresh {
		path, err := kubeconfig.KubeconfigPath(instanceName, cluster.ID)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read existing kubeconfig: %w", err)
		}
		previous = string(data)
	}

	content, err := client.GenerateKubeconfig(context.Background(), cluster.ID)
	if err != nil {
		return handleRancherError(instanceName, err)
	}

	if err := kubeconfig.Save(instanceName, cluster.ID, cluster.Name, content); err != nil {
		return err
	}

	path, err := kubeconfig.KubeconfigPath(instanceName, cluster.ID)
	if err != nil {
		return err
	}
	configPath, err := kubeconfigPath()
	if err != nil {
		return err
	}
	contextName := mergeContextName(instanceName, cluster.Name)

	if err := mergeKubeconfig(instanceName, cluster.Name, content); err != nil {
		return err
	}

	if refresh {
		changed := previous != content
		hint := kubeconfig.TokenExpiryHint(content)
		if changed {
			msg := fmt.Sprintf("kubeconfig for %q updated and merged as context %q", cluster.Name, contextName)
			if hint != "" {
				msg += ", " + hint
			}
			fprintln(cmd.OutOrStdout(), msg)
		} else {
			fprint(cmd.OutOrStdout(), "kubeconfig for %q unchanged, merged context %q", cluster.Name, contextName)
			if hint != "" {
				fprint(cmd.OutOrStdout(), " (%s)", hint)
			}
			fprintln(cmd.OutOrStdout())
		}
		return nil
	}

	reportKubeconfigSaved(cmd.OutOrStdout(), isInteractive(cmd), cluster.Name, path, contextName, configPath)
	return nil
}
