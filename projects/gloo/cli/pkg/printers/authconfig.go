package printers

import (
	"io"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/cliutils"
)

func PrintAuthConfigs(authConfigs extauthv1.AuthConfigList, outputType OutputType) error {
	if outputType == KUBE_YAML || outputType == YAML {
		return PrintKubeCrdList(authConfigs.AsInputResources(), extauthv1.AuthConfigCrd)
	}
	return cliutils.PrintList(outputType.String(), "", authConfigs,
		func(data interface{}, w io.Writer) error {
			AuthConfig(data.(extauthv1.AuthConfigList), w)
			return nil
		}, os.Stdout)
}

// prints AuthConfigs using tables to io.Writer
func AuthConfig(list extauthv1.AuthConfigList, w io.Writer) {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"AuthConfig", "Type"})

	for _, authConfig := range list {
		var authTypes []string
		name := authConfig.GetMetadata().GetName()
		for _, conf := range authConfig.GetConfigs() {
			var authType string
			switch conf.GetAuthConfig().(type) {
			case *extauthv1.AuthConfig_Config_BasicAuth:
				authType = "Basic Auth"
			case *extauthv1.AuthConfig_Config_Oauth:
				authType = "Oauth"
			case *extauthv1.AuthConfig_Config_ApiKeyAuth:
				authType = "ApiKey"
			case *extauthv1.AuthConfig_Config_PluginAuth:
				authType = "Plugin"
			case *extauthv1.AuthConfig_Config_OpaAuth:
				authType = "OPA"
			case *extauthv1.AuthConfig_Config_Ldap:
				authType = "LDAP"
			case *extauthv1.AuthConfig_Config_PassThroughAuth:
				authType = "Passthrough GRPC"
			case *extauthv1.AuthConfig_Config_HmacAuth:
				authType = "HMAC"
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
