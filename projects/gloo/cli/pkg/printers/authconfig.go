package printers

import (
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	v12 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	"github.com/solo-io/go-utils/cliutils"
)

func PrintAuthConfigs(authConfigs v12.AuthConfigList, outputType OutputType) error {
	if outputType == KUBE_YAML || outputType == YAML {
		return PrintKubeCrdList(authConfigs.AsInputResources(), v12.AuthConfigCrd)
	}
	return cliutils.PrintList(outputType.String(), "", authConfigs,
		func(data interface{}, w io.Writer) error {
			AuthConfig(data.(v12.AuthConfigList), w)
			return nil
		}, os.Stdout)
}

// prints AuthConfigs using tables to io.Writer
func AuthConfig(list v12.AuthConfigList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"AuthConfig", "Type"})

	for _, authConfig := range list {
		var authTypes []string
		name := authConfig.GetMetadata().Name
		for _, conf := range authConfig.Configs {
			var authType string
			switch conf.AuthConfig.(type) {
			case *v12.AuthConfig_Config_BasicAuth:
				authType = "Basic Auth"
			case *v12.AuthConfig_Config_Oauth:
				authType = "Oauth"
			case *v12.AuthConfig_Config_ApiKeyAuth:
				authType = "ApiKey"
			case *v12.AuthConfig_Config_PluginAuth:
				authType = "Plugin"
			case *v12.AuthConfig_Config_OpaAuth:
				authType = "OPA"
			case *v12.AuthConfig_Config_Ldap:
				authType = "LDAP"
			default:
				authType = "unknown"
			}
			authTypes = append(authTypes, authType)
		}
		if len(authTypes) == 0 {
			authTypes = []string{"N/A"}
		}
		table.Append([]string{name, strings.Join(authTypes, ",")})
	}

	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.Render()
}
