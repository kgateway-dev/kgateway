package azure

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
)

const profileStringTemplate = `<publishData><publishProfile profileName="{{ .AppName }} - Web Deploy" publishMethod="MSDeploy" publishUrl="{{ .AppName }}.scm.azurewebsites.net:443" msdeploySite="{{ .AppName }}" userName="${{ .AppName }}" userPWD="{{ .Password }}" destinationAppUrl="http://{{ .AppName }}.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile><publishProfile profileName="{{ .AppName }} - FTP" publishMethod="FTP" publishUrl="ftp://waws-prod-dm1-063.ftp.azurewebsites.windows.net/site/wwwroot" ftpPassiveMode="True" userName="{{ .AppName }}\${{ .AppName }}" userPWD="{{ .Password }}" destinationAppUrl="http://{{ .AppName }}.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile></publishData>`

func TestAzure(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Azure Suite")
}
