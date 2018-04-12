package azure

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

var _ = Describe("AzurePublicProfile", func() {
	It("gets the user password from the xml in the secret", func() {
		ref := "secret_ref"
		us := &v1.Upstream{
			Name: "whatever",
			Metadata: &v1.Metadata{
				Annotations: map[string]string{annotationKey: ref},
			},
		}
		secrets := secretwatcher.SecretMap{ref: {Ref: ref, Data: map[string]string{
			publishProfileKey: profileString,
		}}}
		pass, err := getUserPassword(us, secrets)
		Expect(err).NotTo(HaveOccurred())
		Expect(pass).To(Equal("PASSW0RD"))
	})
})

const profileString = `<publishData><publishProfile profileName="appName - Web Deploy" publishMethod="MSDeploy" publishUrl="appName.scm.azurewebsites.net:443" msdeploySite="appName" userName="$appName" userPWD="PASSW0RD" destinationAppUrl="http://appName.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile><publishProfile profileName="appName - FTP" publishMethod="FTP" publishUrl="ftp://waws-prod-dm1-063.ftp.azurewebsites.windows.net/site/wwwroot" ftpPassiveMode="True" userName="appName\$appName" userPWD="PASSW0RD" destinationAppUrl="http://appName.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile></publishData>`
