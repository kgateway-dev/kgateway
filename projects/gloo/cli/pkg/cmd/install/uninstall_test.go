package install_test

import (
	"bytes"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	installutil "github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install/mocks"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

var _ = Describe("Uninstall", func() {
	var (
		ctrl                   *gomock.Controller
		mockHelmClient         *mocks.MockHelmClient
		mockHelmUninstallation *mocks.MockHelmUninstallation
		mockReleaseListRunner  *mocks.MockHelmReleaseListRunner
		crdName                = "authconfigs.enterprise.gloo.solo.io"

		testCRD = `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: ` + crdName + `
spec:
  group: enterprise.gloo.solo.io
  names:
    kind: AuthConfig
    listKind: AuthConfigList
    plural: authconfigs
    shortNames:
      - ac
    singular: authconfig
  scope: Namespaced
  version: v1
  versions:
    - name: v1
      served: true
      storage: true
`
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		mockHelmClient = mocks.NewMockHelmClient(ctrl)
		mockHelmUninstallation = mocks.NewMockHelmUninstallation(ctrl)
		mockReleaseListRunner = mocks.NewMockHelmReleaseListRunner(ctrl)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("uninstalls cleanly by default", func() {
		mockReleaseListRunner.EXPECT().
			Run().
			Return([]*release.Release{{
				Name: constants.GlooReleaseName,
			}}, nil)
		mockReleaseListRunner.EXPECT().
			SetFilter(constants.GlooReleaseName)

		mockHelmClient.EXPECT().
			ReleaseList(defaults.GlooSystem).
			Return(mockReleaseListRunner, nil)
		mockHelmClient.EXPECT().
			NewUninstall(defaults.GlooSystem).
			Return(mockHelmUninstallation, nil)
		mockHelmUninstallation.EXPECT().
			Run(constants.GlooReleaseName).
			Return(nil, nil)

		outputBuffer := new(bytes.Buffer)

		uninstaller := install.NewUninstallerWithOutput(mockHelmClient, installutil.NewMockKubectl([]string{}, []string{}), outputBuffer)
		err := uninstaller.Uninstall(&options.Options{
			Uninstall: options.Uninstall{Namespace: defaults.GlooSystem},
		})

		Expect(err).NotTo(HaveOccurred())
	})

	It("can uninstall CRDs when requested", func() {
		mockReleaseListRunner.EXPECT().
			Run().
			Return([]*release.Release{{
				Name: constants.GlooReleaseName,
				Chart: &chart.Chart{
					Files: []*chart.File{{
						Name: "crds/crdA.yaml",
						Data: []byte(testCRD),
					}},
				},
			}}, nil).
			Times(2)
		mockReleaseListRunner.EXPECT().
			SetFilter(constants.GlooReleaseName)

		mockHelmClient.EXPECT().
			ReleaseList(defaults.GlooSystem).
			Return(mockReleaseListRunner, nil).
			Times(2)
		mockHelmClient.EXPECT().
			NewUninstall(defaults.GlooSystem).
			Return(mockHelmUninstallation, nil)
		mockHelmUninstallation.EXPECT().
			Run(constants.GlooReleaseName).
			Return(nil, nil)

		outputBuffer := new(bytes.Buffer)

		mockKubectl := installutil.NewMockKubectl([]string{
			"delete crd " + crdName,
		}, []string{})

		uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, outputBuffer)
		err := uninstaller.Uninstall(&options.Options{
			Uninstall: options.Uninstall{
				Namespace:  defaults.GlooSystem,
				DeleteCrds: true,
			},
		})

		Expect(err).NotTo(HaveOccurred())
	})
})
