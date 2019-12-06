package install_test

import (
	"bytes"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
)

var _ = Describe("Install", func() {
	var (
		mockHelmClient       *mocks.MockHelmClient
		mockHelmInstallation *mocks.MockHelmInstallation
		ctrl                 *gomock.Controller

		glooOsVersion   = "v1.0.0"
		glooOsChartUri  = "https://storage.googleapis.com/solo-public-helm/charts/gloo-v1.0.0.tgz"
		testCrdContent  = "test-crd-content"
		testHookContent = `
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "5" # must be executed before cert-gen job
subjects:
- kind: ServiceAccount
  name: gateway-certgen
  namespace: gloo-system
roleRef:
  kind: ClusterRole
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  apiGroup: rbac.authorization.k8s.io
`
		testCleanupHook = `
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-vwc-update-gloo-system
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": post-install,post-upgrade
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
    "` + constants.HookCleanupResourceAnnotation + `": "true" # Used internally to mark "hook cleanup" resources
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "get", "update"]
- apiGroups: ["admissionregistration.k8s.io"]
  resources: ["validatingwebhookconfigurations"]
  verbs: ["get", "update"]
`

		chart = &helmchart.Chart{
			Metadata: &helmchart.Metadata{
				Name: "gloo-installer-test-chart",
			},
			Files: []*helmchart.File{{
				Name: "crds/crdA.yaml",
				Data: []byte(testCrdContent),
			}},
		}

		helmRelease = &release.Release{
			Chart: chart,
			Hooks: []*release.Hook{
				{
					Manifest: testHookContent,
				},
				{
					Manifest: testCleanupHook,
				},
			},
			Namespace: defaults.GlooSystem,
		}
	)

	BeforeEach(func() {
		version.Version = glooOsVersion

		ctrl = gomock.NewController(GinkgoT())
		mockHelmClient = mocks.NewMockHelmClient(ctrl)
		mockHelmInstallation = mocks.NewMockHelmInstallation(ctrl)
	})

	AfterEach(func() {
		version.Version = version.UndefinedVersion
		ctrl.Finish()
	})

	It("installs cleanly by default", func() {
		installConfig := &options.Install{
			Namespace:       defaults.GlooSystem,
			HelmReleaseName: constants.GlooReleaseName,
		}

		helmEnv := &cli.EnvSettings{
			KubeConfig: "path-to-kube-config",
		}

		mockHelmInstallation.EXPECT().
			Run(chart, map[string]interface{}{}).
			Return(helmRelease, nil)

		mockHelmClient.EXPECT().
			NewInstall(defaults.GlooSystem, installConfig.HelmReleaseName, installConfig.DryRun).
			Return(mockHelmInstallation, helmEnv, nil)

		mockHelmClient.EXPECT().
			DownloadChart(glooOsChartUri).
			Return(chart, nil)

		mockHelmClient.EXPECT().
			ReleaseExists(defaults.GlooSystem, constants.GlooReleaseName).
			Return(false, nil)

		dryRunOutputBuffer := new(bytes.Buffer)

		installer := install.NewInstallerWithWriter(mockHelmClient, dryRunOutputBuffer)
		err := installer.Install(&install.InstallerConfig{
			InstallCliArgs: installConfig,
		})

		Expect(err).NotTo(HaveOccurred(), "No error should result from the installation")
		Expect(dryRunOutputBuffer.String()).To(BeEmpty())
	})

	It("outputs the expected kinds when in a dry run", func() {
		installConfig := &options.Install{
			Namespace:       defaults.GlooSystem,
			HelmReleaseName: constants.GlooReleaseName,
			DryRun:          true,
		}

		helmEnv := &cli.EnvSettings{
			KubeConfig: "path-to-kube-config",
		}

		mockHelmInstallation.EXPECT().
			Run(chart, map[string]interface{}{}).
			Return(helmRelease, nil)

		mockHelmClient.EXPECT().
			NewInstall(defaults.GlooSystem, installConfig.HelmReleaseName, installConfig.DryRun).
			Return(mockHelmInstallation, helmEnv, nil)

		mockHelmClient.EXPECT().
			DownloadChart(glooOsChartUri).
			Return(chart, nil)

		dryRunOutputBuffer := new(bytes.Buffer)
		installer := install.NewInstallerWithWriter(mockHelmClient, dryRunOutputBuffer)

		err := installer.Install(&install.InstallerConfig{
			InstallCliArgs: installConfig,
		})

		Expect(err).NotTo(HaveOccurred(), "No error should result from the installation")

		dryRunOutput := dryRunOutputBuffer.String()

		Expect(dryRunOutput).To(ContainSubstring(testCrdContent), "Should output CRD definitions")
		Expect(dryRunOutput).NotTo(ContainSubstring(constants.HookCleanupResourceAnnotation), "Should not output cleanup hooks")
		Expect(dryRunOutput).To(ContainSubstring("helm.sh/hook"), "Should output non-cleanup hooks")
	})
})