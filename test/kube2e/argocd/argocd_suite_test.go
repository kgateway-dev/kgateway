package argocd_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/test/helpers"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestArgoCD(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "ArgoCD Suite")
}

var _ = BeforeSuite(func() {
	installArgoCD()
	deployHelmRepo()
})

var _ = AfterSuite(func() {
	uninstallArgoCD()
})

func installArgoCD() {
	fmt.Println("Installing ArgoCD")
	// helm repo add argo https://argoproj.github.io/argo-helm
	runAndCleanCommand("helm", "repo", "add", "argo", "https://argoproj.github.io/argo-helm")

	// helm install argocd argo/argo-cd --wait
	runAndCleanCommand("helm", "install", "argocd", "argo/argo-cd", "--wait")
}

func uninstallArgoCD() {
	fmt.Println("Uninstalling ArgoCD")
	uninstallGloo()

	// helm uninstall argocd --wait
	runAndCleanCommand("helm", "uninstall", "argocd", "--wait")
}

func deployHelmRepo() {
	fmt.Println("Deploying helm repo")
	// ./deploy-helm-server.sh
	runAndCleanCommand("./deploy-helm-server.sh")
}
