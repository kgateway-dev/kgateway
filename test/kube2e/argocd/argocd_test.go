package argocd_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/testutils/helper"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

const namespace = defaults.GlooSystem

var _ = Describe("Kube2e: ArgoCD", func() {

	var (
		testHelper *helper.SoloTestHelper
	)

	BeforeEach(func() {

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../..")
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("Tests the Lifecycle", func() {
		installGloo()
		checkGlooHealthy(testHelper)

		// Sync once more to simulate an upgrade
		syncGloo()
		checkGlooHealthy(testHelper)

		uninstallGloo()
	})
})

func installGloo() {
	uninstallGloo()

	repo := "http://helm-repo/"

	// argocd --core app create gloo \
	// --repo http://helm-repo/ --helm-chart gloo --revision $VERSION \
	// --dest-namespace gloo-system --dest-server https://kubernetes.default.svc \
	// --sync-option CreateNamespace=true --upsert --values-literal-file helm-override.yaml
	command := []string{"--core", "app", "create", "gloo",
		"--repo", repo, "--helm-chart", "gloo", "--revision", version,
		"--dest-namespace", "gloo-system", "--dest-server", "https://kubernetes.default.svc",
		"--sync-option", "CreateNamespace=true", "--upsert", "--values-literal-file", "helm-override.yaml"}
	fmt.Printf("Running argo command : %s\n", command)
	runAndCleanCommand("argocd", command...)

	syncGloo()
}

func syncGloo() {
	// argocd --core app sync gloo
	command := []string{"--core", "app", "sync", "gloo"}
	fmt.Printf("Running argo command : %s\n", command)
	runAndCleanCommand("argocd", command...)
}

func uninstallGloo() {
	// argocd --core app delete gloo -y
	cmd := exec.Command("argocd", "--core", "app", "delete", "gloo", "-y")
	cmd.Output()
}

func checkRolloutJobDeleted() {
	EventuallyWithOffset(1, func() string {
		cmd := exec.Command("kubectl", "-n", "gloo-system", "get", "jobs", "-A")
		b, err := cmd.Output()
		Expect(err).To(BeNil())
		return string(b)
	}, "60s", "10s").ShouldNot(
		ContainSubstring("gloo-resource-rollout "))
}

func checkGlooHealthyAndSyncedInArgo() {
	// Get the state of gloo
	// argocd app get gloo --hard-refresh -o json | jq '.status.health.status'
	fmt.Println("Checking if gloo is healthy")
	EventuallyWithOffset(1, func() string {
		command := "argocd app get gloo --hard-refresh -o json | jq '.status.health.status'"
		cmd := exec.Command("bash", "-c", command)
		b, err := cmd.Output()
		Expect(err).To(BeNil())
		return string(b)
	}).Should(
		ContainSubstring("Healthy"))
	// argocd app get gloo --hard-refresh -o json | jq '.status.sync.status'
	fmt.Println("Checking if gloo is synced")
	EventuallyWithOffset(1, func() string {
		command := "argocd app get gloo --hard-refresh -o json | jq '.status.sync.status'"
		cmd := exec.Command("bash", "-c", command)
		b, err := cmd.Output()
		Expect(err).To(BeNil())
		return string(b)
	}).Should(
		ContainSubstring("Synced"))
}

func runAndCleanCommand(name string, arg ...string) []byte {
	ctx, _ := context.WithTimeout(context.TODO(), 300*time.Second)
	cmd := exec.CommandContext(ctx, name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*exec.ExitError); ok {
			fmt.Println("ExitError: ", v.Error())
		}
	}
	Expect(err).To(BeNil())
	cmd.Process.Kill()
	cmd.Process.Release()
	return b
}

func checkGlooHealthy(testHelper *helper.SoloTestHelper) {
	// Wait until the resource rollout job has been deleted to ensure that argo does not go out of sync
	checkRolloutJobDeleted()

	checkGlooHealthyAndSyncedInArgo()
	deploymentNames := []string{"gloo", "discovery", "gateway-proxy"}
	for _, deploymentName := range deploymentNames {
		runAndCleanCommand("kubectl", "rollout", "status", "deployment", "-n", testHelper.InstallNamespace, deploymentName)
	}
	kube2e.GlooctlCheckEventuallyHealthy(2, testHelper, "90s")
}
