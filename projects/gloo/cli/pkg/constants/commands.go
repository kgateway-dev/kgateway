package constants

import (
	"github.com/rotisserie/eris"
	"github.com/spf13/cobra"
)

var (
	SubcommandError = eris.New("please select a subcommand")
)

var (
	VIRTUAL_SERVICE_COMMAND = cobra.Command{
		Use:     "virtualservice",
		Aliases: []string{"vs", "virtualservices"},
	}

	ROUTE_TABLE_COMMAND = cobra.Command{
		Use:     "routetable",
		Aliases: []string{"rt", "routetables"},
	}

	UPSTREAM_COMMAND = cobra.Command{
		Use:     "upstream",
		Aliases: []string{"u", "us", "upstreams"},
	}

	UPSTREAM_GROUP_COMMAND = cobra.Command{
		Use:     "upstreamgroup",
		Aliases: []string{"ug", "ugs", "upstreamgroups"},
	}

	PROXY_COMMAND = cobra.Command{
		Use:     "proxy",
		Aliases: []string{"p", "proxies"},
	}

	ROUTE_COMMAND = cobra.Command{
		Use:     "route",
		Aliases: []string{"r", "routes"},
	}

	SECRET_COMMAND = cobra.Command{
		Use:     "secret",
		Aliases: []string{"s", "secrets"},
	}

	AUTH_CONFIG_COMMAND = cobra.Command{
		Use:     "authconfig",
		Aliases: []string{"ac", "authconfigs"},
	}

	RATELIMIT_CONFIG_COMMAND = cobra.Command{
		Use:     "ratelimitconfig",
		Aliases: []string{"rlc", "ratelimitconfigs"},
	}

	ADD_COMMAND = cobra.Command{
		Use:     "add",
		Aliases: []string{"a"},
		Short:   "Adds configuration to a top-level Gloo resource",
	}

	CHECK_COMMAND = cobra.Command{
		Use:   "check",
		Short: "Checks Gloo resources for errors (requires Gloo running on Kubernetes)",
	}

	CHECK_CRD_COMMAND = cobra.Command{
		Use:   "check-crds",
		Short: "Checks Gloos CRDs for consistency against an official (or local) helm charts CRDs",
	}

	CREATE_COMMAND = cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Create a Gloo resource",
		Long:    "Gloo resources be created from files (including stdin)",
	}

	DEBUG_COMMAND = cobra.Command{
		Use:   "debug",
		Short: "Debug a Gloo resource (requires Gloo running on Kubernetes)",
	}

	DEBUG_LOG_COMMAND = cobra.Command{
		Use:     "logs",
		Aliases: []string{"log"},
		Short:   "Debug Gloo logs (requires Gloo running on Kubernetes)",
	}

	DEBUG_YAML_COMMAND = cobra.Command{
		Use:   "yaml",
		Short: "Dump YAML representing the current Gloo state (requires Gloo running on Kubernetes)",
	}

	DELETE_COMMAND = cobra.Command{
		Use:     "delete",
		Aliases: []string{"d"},
		Short:   "Delete a Gloo resource",
	}

	DEMO_COMMAND = cobra.Command{
		Use:   "demo",
		Short: "Demos (requires 4 tools to be installed and accessible via the PATH: glooctl, kubectl, docker, and kind.)",
	}

	DEMO_FEDERATION_COMMAND = cobra.Command{
		Use:   "federation",
		Short: "Bootstrap a multicluster demo with Gloo Federation.",
		Long: "Running the Gloo Federation demo setup locally requires 4 tools to be installed and accessible via the " +
			"PATH: glooctl, kubectl, docker, and kind. This command will bootstrap 2 kind clusters, one of which will run " +
			"the Gloo Federation management-plane as well as Gloo Enterprise, and the other will just run Gloo. " +
			"Please note that cluster registration will only work on darwin and linux OS.",
	}

	GET_COMMAND = cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Display one or a list of Gloo resources",
	}

	INSTALL_COMMAND = cobra.Command{
		Use:   "install",
		Short: "install gloo on different platforms",
		Long:  "choose which version of Gloo to install.",
	}

	UNINSTALL_COMMAND = cobra.Command{
		Use:   "uninstall",
		Short: "uninstall gloo",
	}

	UNINSTALL_GLOO_FED_COMMAND = cobra.Command{
		Use:   "federation",
		Short: "uninstall gloo federation",
	}

	UPGRADE_COMMAND = cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"ug"},
		Short:   "upgrade glooctl binary",
	}

	EDIT_COMMAND = cobra.Command{
		Use:     "edit",
		Aliases: []string{"ed"},
		Short:   "Edit a Gloo resource",
	}

	OBSERVABILITY_COMMAND = cobra.Command{
		Use:     "observability",
		Aliases: []string{"o", "obs", "observe"},
		Short:   "root command for observability functionality",
	}

	SETTINGS_COMMAND = cobra.Command{
		Use:     "settings",
		Aliases: []string{"st", "set"},
		Short:   "root command for settings",
	}

	CONFIG_EXTAUTH_COMMAND = cobra.Command{
		Use:     "externalauth",
		Aliases: []string{"extauth"},
		Short:   "root command for external auth functionality",
	}

	CONFIG_RATELIMIT_COMMAND = cobra.Command{
		Use:   "ratelimit",
		Short: "root command for rate limit functionality",
	}

	VERSION_COMMAND = cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Print current version",
		Long:    "Get the version of Glooctl and Gloo",
	}

	DASHBOARD_COMMAND = cobra.Command{
		Use:     "dashboard",
		Aliases: []string{"ui"},
		Short:   "Open Gloo dashboard",
		Long:    "Open the Gloo dashboard/UI in your default browser",
	}

	CLUSTER_COMMAND = cobra.Command{
		Use:   "cluster",
		Short: "Cluster commands",
		Long:  "Commands related to managing multiple clusters",
	}

	CLUSTER_LIST_COMMAND = cobra.Command{
		Use:   "list",
		Short: "List clusters registered to the Gloo Federation control plane",
	}

	CLUSTER_REGISTER_COMMAND = cobra.Command{
		Use:   "register",
		Short: "Register a cluster to the Gloo Federation control plane",
		Long:  "Register a cluster to the Gloo Federation control plane. Registered clusters can be targeted for discovery and configuration.",
	}

	CLUSTER_DEREGISTER_COMMAND = cobra.Command{
		Use:   "deregister",
		Short: "Deregister a cluster to the Gloo Federation control plane",
		Long: "Deregister a cluster from the Gloo Federation control plane. Deregistered clusters can no longer be " +
			"targeted for discovery and configuration. This will not delete the cluster or the managing namespace, but it " +
			"will delete the service account, cluster role, and cluster role binding created on the remote cluster " +
			"during the cluster registration process.",
	}

	PLUGIN_COMMAND = cobra.Command{
		Use:   "plugin",
		Short: "Commands for interacting with glooctl plugins",
		Long: "Commands for interacting with glooctl plugins. Glooctl plugins are arbitrary binary executables " +
			"in your path with the prefix 'glooctl-'.",
	}

	PLUGIN_LIST_COMMAND = cobra.Command{
		Use:   "list",
		Short: "List available glooctl plugins",
	}

	ISTIO_COMMAND = cobra.Command{
		Use:   "istio",
		Short: "Commands for interacting with Istio in Gloo",
	}
)
