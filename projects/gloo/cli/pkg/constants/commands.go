package constants

import (
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
)

var (
	SubcommandError = errors.New("please select a subcommand")
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
		Aliases: []string{"s", "secret"},
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

	DELETE_COMMAND = cobra.Command{
		Use:     "delete",
		Aliases: []string{"d"},
		Short:   "Delete a Gloo resource",
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
)
