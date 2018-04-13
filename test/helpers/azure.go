package helpers

import (
	"bytes"
	"os"
	"text/template"

	. "github.com/onsi/ginkgo"
	"github.com/pkg/errors"
)

const AzureProfileStringTemplate = `<publishData><publishProfile profileName="{{ .AppName }} - Web Deploy" publishMethod="MSDeploy" publishUrl="{{ .AppName }}.scm.azurewebsites.net:443" msdeploySite="{{ .AppName }}" userName="${{ .AppName }}" userPWD="{{ .Password }}" destinationAppUrl="http://{{ .AppName }}.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile><publishProfile profileName="{{ .AppName }} - FTP" publishMethod="FTP" publishUrl="ftp://waws-prod-dm1-063.ftp.azurewebsites.windows.net/site/wwwroot" ftpPassiveMode="True" userName="{{ .AppName }}\${{ .AppName }}" userPWD="{{ .Password }}" destinationAppUrl="http://{{ .AppName }}.azurewebsites.net" SQLServerDBConnectionString="" mySQLDBConnectionString="" hostingProviderForumLink="" controlPanelLink="http://windows.azure.com" webSystem="WebSites"><databases /></publishProfile></publishData>`

func AzureProfileString() string {
	data := struct {
		AppName  string
		Password string
	}{
		AppName:  os.Getenv("AZURE_FUNCTION_APP"),
		Password: os.Getenv("AZURE_PROFILE_PASSWORD"),
	}
	if data.AppName == "" || data.Password == "" {
		Skip("must set AZURE_FUNCTION_APP and AZURE_PROFILE_PASSWORD to run this test")
	}
	tmpl, err := template.New("ProfileString").Parse(AzureProfileStringTemplate)
	if err != nil {
		panic(errors.Wrap(err, "parsing template"))
	}
	buf := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, "ProfileString", data); err != nil {
		panic(errors.Wrap(err, "executing template"))
	}

	return buf.String()
}
