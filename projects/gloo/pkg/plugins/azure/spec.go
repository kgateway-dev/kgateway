package azure

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/azure"
)

func GetHostname(s *azure.UpstreamSpec) string {
	return fmt.Sprintf("%s.azurewebsites.net", s.GetFunctionAppName())
}
