package install_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil/testutil"
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
		crdDeleteCmd           string
		crdName                = "authconfigs.enterprise.gloo.solo.io"
		ctx                    context.Context
		cancel                 context.CancelFunc

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

		ctx, cancel = context.WithCancel(context.Background())
		mockHelmClient = mocks.NewMockHelmClient(ctrl)
		mockHelmUninstallation = mocks.NewMockHelmUninstallation(ctrl)
		mockReleaseListRunner = mocks.NewMockHelmReleaseListRunner(ctrl)

		crdDeleteCmd = fmt.Sprintf("delete crd %s", strings.Join(install.GlooCrdNames, " "))
	})

	AfterEach(func() {
		ctrl.Finish()
		cancel()
	})

	When("a Gloo release object exists", func() {

		BeforeEach(func() {
			mockHelmClient.EXPECT().NewUninstall(defaults.GlooSystem).Return(mockHelmUninstallation, nil)
			mockHelmClient.EXPECT().ReleaseExists(defaults.GlooSystem, constants.GlooReleaseName).Return(true, nil)
			mockHelmClient.EXPECT().ReleaseList(defaults.GlooSystem).Return(mockReleaseListRunner, nil).MaxTimes(1)
			mockReleaseListRunner.EXPECT().Run().Return([]*release.Release{{
				Name: constants.GlooReleaseName,
				Chart: &chart.Chart{
					Files: []*chart.File{{
						Name: "crds/crdA.yaml",
						Data: []byte(testCRD),
					}},
				},
			}}, nil).MaxTimes(1)
			mockHelmUninstallation.EXPECT().Run(constants.GlooReleaseName).Return(nil, nil)
		})

		It("can uninstall", func() {
			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, testutil.NewMockKubectl([]string{}, []string{}), new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
			}, install.Gloo)

			Expect(err).NotTo(HaveOccurred())
		})

		It("can uninstall CRDs when requested", func() {
			mockKubectl := testutil.NewMockKubectl([]string{"delete crd " + crdName}, []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteCrds:      true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})

		It("can remove namespace when requested", func() {
			mockKubectl := testutil.NewMockKubectl([]string{
				"delete namespace " + defaults.GlooSystem,
			}, []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteNamespace: true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})

		It("--all flag behaves as expected", func() {
			mockKubectl := testutil.NewMockKubectl([]string{
				"delete crd " + crdName,
				"delete namespace " + defaults.GlooSystem,
			}, []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteAll:       true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("no Gloo release object exists", func() {

		var (
			namespacedDeleteCmds,
			clusterScopedDeleteCmds []string
		)

		BeforeEach(func() {
			namespacedDeleteCmds, clusterScopedDeleteCmds = nil, nil // important!

			mockHelmClient.EXPECT().ReleaseExists(defaults.GlooSystem, constants.GlooReleaseName).Return(false, nil)

			glooAppFlags := install.LabelsToFlagString(install.GlooComponentLabels)
			for _, kind := range install.GlooNamespacedKinds {
				namespacedDeleteCmds = append(namespacedDeleteCmds,
					fmt.Sprintf("delete %s -n %s -l %s", kind, defaults.GlooSystem, glooAppFlags))
			}
			for _, kind := range install.GlooClusterScopedKinds {
				clusterScopedDeleteCmds = append(clusterScopedDeleteCmds,
					fmt.Sprintf("delete %s -l %s", kind, glooAppFlags))
			}
		})

		It("deletes all resources with the app=gloo label in the given namespace", func() {
			mockKubectl := testutil.NewMockKubectl(namespacedDeleteCmds, []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
			}, install.Gloo)

			Expect(err).NotTo(HaveOccurred())
		})

		It("removes the Gloo CRDs when the appropriate flag is provided", func() {
			mockKubectl := testutil.NewMockKubectl(append(namespacedDeleteCmds, crdDeleteCmd), []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteCrds:      true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})

		It("removes namespace when the appropriate flag is provided", func() {
			mockKubectl := testutil.NewMockKubectl(append(namespacedDeleteCmds, "delete namespace "+defaults.GlooSystem), []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteNamespace: true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})

		It("--all flag behaves as expected", func() {
			commands := append(namespacedDeleteCmds, clusterScopedDeleteCmds...)
			commands = append(commands, crdDeleteCmd)
			commands = append(commands, "delete namespace "+defaults.GlooSystem)
			mockKubectl := testutil.NewMockKubectl(commands, []string{})

			uninstaller := install.NewUninstallerWithOutput(mockHelmClient, mockKubectl, new(bytes.Buffer))
			err := uninstaller.Uninstall(ctx, &options.HelmUninstall{
				Namespace:       defaults.GlooSystem,
				HelmReleaseName: constants.GlooReleaseName,
				DeleteAll:       true,
			}, install.Gloo)
			Expect(mockKubectl.Next).To(Equal(len(mockKubectl.Expected)))
			Expect(err).NotTo(HaveOccurred())
		})

	})

})
