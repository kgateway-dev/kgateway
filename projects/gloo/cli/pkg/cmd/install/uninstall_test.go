package install_test

import (
	"fmt"
	"io"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/pflag"
)

type MockKubectl struct {
	expected        []string
	next            int
	stdoutLines     []string
	stdoutLineIndex int
}

func NewMockKubectl(cmds []string, stdoutLines []string) *MockKubectl {
	return &MockKubectl{
		expected:    cmds,
		next:        0,
		stdoutLines: stdoutLines,
	}
}

func (k *MockKubectl) Kubectl(stdin io.Reader, args ...string) error {
	// If this fails then the CLI tried to run commands we didn't account for in the mock
	Expect(k.next < len(k.expected)).To(BeTrue())
	Expect(stdin).To(BeNil())
	cmd := strings.Join(args, " ")
	Expect(cmd).To(BeEquivalentTo(k.expected[k.next]))
	k.next = k.next + 1
	return nil
}

func (k *MockKubectl) KubectlOut(stdin io.Reader, args ...string) ([]byte, error) {
	Expect(k.next < len(k.expected)).To(BeTrue())
	Expect(stdin).To(BeNil())
	cmd := strings.Join(args, " ")
	Expect(cmd).To(BeEquivalentTo(k.expected[k.next]))
	k.next = k.next + 1
	Expect(k.stdoutLineIndex < len(k.stdoutLines)).To(BeTrue(), "Mock kubectl has run out of stdout lines on command "+cmd)
	stdOutLine := k.stdoutLines[k.stdoutLineIndex]
	k.stdoutLineIndex = k.stdoutLineIndex + 1
	return []byte(stdOutLine), nil
}

var _ = Describe("Uninstall", func() {

	const (
		deleteCrds    = `delete crd gateways.gateway.solo.io proxies.gloo.solo.io settings.gloo.solo.io upstreams.gloo.solo.io upstreamgroups.gloo.solo.io virtualservices.gateway.solo.io routetables.gateway.solo.io authconfigs.enterprise.gloo.solo.io`
		testInstallId = "test-install-id"

		// expects to be formatted with the namespace
		findInstallIdCmd = "-n %s get deployment -l gloo=gloo -ojsonpath='{.items[0].metadata.labels.installationId}'"
	)

	var flagSet *pflag.FlagSet
	var opts options.Options

	BeforeEach(func() {
		flagSet = pflag.NewFlagSet("uninstall", pflag.ContinueOnError)
		opts = options.Options{}
		flagutils.AddUninstallFlags(flagSet, &opts.Uninstall)
	})

	uninstall := func(cli *MockKubectl) error {
		err := install.UninstallGloo(&opts, cli)
		// If this fails, then the mock CLI had extra commands that were expected to run but weren't
		Expect(cli.next).To(BeEquivalentTo(len(cli.expected)))

		return err
	}

	It("works with no args", func() {
		flagSet.Parse([]string{})
		commands := []string{
			fmt.Sprintf(findInstallIdCmd, "gloo-system"),
			"delete Deployment -l app=glooe-grafana -n gloo-system",
			"delete Deployment -l app=glooe-prometheus -n gloo-system",
			"delete Deployment -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete Service -l app=glooe-grafana -n gloo-system",
			"delete Service -l app=glooe-prometheus -n gloo-system",
			"delete Service -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete ServiceAccount -l app=glooe-grafana -n gloo-system",
			"delete ServiceAccount -l app=glooe-prometheus -n gloo-system",
			"delete ServiceAccount -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete ConfigMap -l app=glooe-grafana -n gloo-system",
			"delete ConfigMap -l app=glooe-prometheus -n gloo-system",
			"delete ConfigMap -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete Job -l app=glooe-grafana -n gloo-system",
			"delete Job -l app=glooe-prometheus -n gloo-system",
			"delete Job -l app=gloo,installationId=test-install-id -n gloo-system",
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(commands, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with namespace", func() {
		flagSet.Parse([]string{"-n", "foo"})
		cmds := []string{
			fmt.Sprintf(findInstallIdCmd, "foo"),
			"delete Deployment -l app=glooe-grafana -n foo",
			"delete Deployment -l app=glooe-prometheus -n foo",
			"delete Deployment -l app=gloo,installationId=test-install-id -n foo",
			"delete Service -l app=glooe-grafana -n foo",
			"delete Service -l app=glooe-prometheus -n foo",
			"delete Service -l app=gloo,installationId=test-install-id -n foo",
			"delete ServiceAccount -l app=glooe-grafana -n foo",
			"delete ServiceAccount -l app=glooe-prometheus -n foo",
			"delete ServiceAccount -l app=gloo,installationId=test-install-id -n foo",
			"delete ConfigMap -l app=glooe-grafana -n foo",
			"delete ConfigMap -l app=glooe-prometheus -n foo",
			"delete ConfigMap -l app=gloo,installationId=test-install-id -n foo",
			"delete Job -l app=glooe-grafana -n foo",
			"delete Job -l app=glooe-prometheus -n foo",
			"delete Job -l app=gloo,installationId=test-install-id -n foo",
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(cmds, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete crds", func() {
		flagSet.Parse([]string{"--delete-crds"})
		cmds := []string{
			fmt.Sprintf(findInstallIdCmd, "gloo-system"),
			"delete Deployment -l app=glooe-grafana -n gloo-system",
			"delete Deployment -l app=glooe-prometheus -n gloo-system",
			"delete Deployment -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete Service -l app=glooe-grafana -n gloo-system",
			"delete Service -l app=glooe-prometheus -n gloo-system",
			"delete Service -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete ServiceAccount -l app=glooe-grafana -n gloo-system",
			"delete ServiceAccount -l app=glooe-prometheus -n gloo-system",
			"delete ServiceAccount -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete ConfigMap -l app=glooe-grafana -n gloo-system",
			"delete ConfigMap -l app=glooe-prometheus -n gloo-system",
			"delete ConfigMap -l app=gloo,installationId=test-install-id -n gloo-system",
			"delete Job -l app=glooe-grafana -n gloo-system",
			"delete Job -l app=glooe-prometheus -n gloo-system",
			"delete Job -l app=gloo,installationId=test-install-id -n gloo-system",
			deleteCrds,
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(cmds, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete crds and namespace", func() {
		flagSet.Parse([]string{"-n", "foo", "--delete-crds"})
		cmds := []string{
			fmt.Sprintf(findInstallIdCmd, "foo"),
			"delete Deployment -l app=glooe-grafana -n foo",
			"delete Deployment -l app=glooe-prometheus -n foo",
			"delete Deployment -l app=gloo,installationId=test-install-id -n foo",
			"delete Service -l app=glooe-grafana -n foo",
			"delete Service -l app=glooe-prometheus -n foo",
			"delete Service -l app=gloo,installationId=test-install-id -n foo",
			"delete ServiceAccount -l app=glooe-grafana -n foo",
			"delete ServiceAccount -l app=glooe-prometheus -n foo",
			"delete ServiceAccount -l app=gloo,installationId=test-install-id -n foo",
			"delete ConfigMap -l app=glooe-grafana -n foo",
			"delete ConfigMap -l app=glooe-prometheus -n foo",
			"delete ConfigMap -l app=gloo,installationId=test-install-id -n foo",
			"delete Job -l app=glooe-grafana -n foo",
			"delete Job -l app=glooe-prometheus -n foo",
			"delete Job -l app=gloo,installationId=test-install-id -n foo",
			deleteCrds,
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(cmds, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete namespace", func() {
		flagSet.Parse([]string{"--delete-namespace"})
		cli := NewMockKubectl([]string{fmt.Sprintf(findInstallIdCmd, "gloo-system"), "delete namespace gloo-system"}, []string{testInstallId})
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete namespace with custom namespace", func() {
		flagSet.Parse([]string{"--delete-namespace", "-n", "foo"})
		cli := NewMockKubectl([]string{fmt.Sprintf(findInstallIdCmd, "foo"), "delete namespace foo"}, []string{testInstallId})
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete namespace and crds", func() {
		flagSet.Parse([]string{"--delete-namespace", "--delete-crds"})
		cli := NewMockKubectl([]string{fmt.Sprintf(findInstallIdCmd, "gloo-system"), "delete namespace gloo-system", deleteCrds}, []string{testInstallId})
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete crds and namespace with custom namespace", func() {
		flagSet.Parse([]string{"--delete-namespace", "--delete-crds", "-n", "foo"})
		cli := NewMockKubectl([]string{fmt.Sprintf(findInstallIdCmd, "foo"), "delete namespace foo", deleteCrds}, []string{testInstallId})
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete all", func() {
		flagSet.Parse([]string{"--all"})
		cmds := []string{
			fmt.Sprintf(findInstallIdCmd, "gloo-system"),
			"delete namespace gloo-system",
			deleteCrds,
			"delete ClusterRole -l app=gloo,installationId=test-install-id",
			"delete ClusterRoleBinding -l app=gloo,installationId=test-install-id",
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(cmds, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	It("works with delete all custom namespace", func() {
		flagSet.Parse([]string{"--all", "-n", "foo"})
		cmds := []string{
			fmt.Sprintf(findInstallIdCmd, "foo"),
			"delete namespace foo",
			deleteCrds,
			"delete ClusterRole -l app=gloo,installationId=test-install-id",
			"delete ClusterRoleBinding -l app=gloo,installationId=test-install-id",
		}
		stdoutLines := []string{testInstallId}
		cli := NewMockKubectl(cmds, stdoutLines)
		err := uninstall(cli)
		Expect(err).NotTo(HaveOccurred(), "The uninstall should be successful")
	})

	When("the install ID is not discoverable", func() {
		It("errors by default", func() {
			flagSet.Parse([]string{})
			commands := []string{
				fmt.Sprintf(findInstallIdCmd, "gloo-system"),
			}
			installId := ""
			cli := NewMockKubectl(commands, []string{installId})
			err := uninstall(cli)
			Expect(err).To(HaveOccurred(), "An error should occur if the install ID is not discoverable")
		})

		It("proceeds and uses the old logic when forced", func() {
			flagSet.Parse([]string{"--force"})
			commands := []string{
				fmt.Sprintf(findInstallIdCmd, "gloo-system"),
				"delete Deployment -l app=glooe-grafana -n gloo-system",
				"delete Deployment -l app=glooe-prometheus -n gloo-system",
				"delete Deployment -l app=gloo -n gloo-system",
				"delete Service -l app=glooe-grafana -n gloo-system",
				"delete Service -l app=glooe-prometheus -n gloo-system",
				"delete Service -l app=gloo -n gloo-system",
				"delete ServiceAccount -l app=glooe-grafana -n gloo-system",
				"delete ServiceAccount -l app=glooe-prometheus -n gloo-system",
				"delete ServiceAccount -l app=gloo -n gloo-system",
				"delete ConfigMap -l app=glooe-grafana -n gloo-system",
				"delete ConfigMap -l app=glooe-prometheus -n gloo-system",
				"delete ConfigMap -l app=gloo -n gloo-system",
				"delete Job -l app=glooe-grafana -n gloo-system",
				"delete Job -l app=glooe-prometheus -n gloo-system",
				"delete Job -l app=gloo -n gloo-system",
			}
			installId := ""
			stdoutLines := []string{installId}
			cli := NewMockKubectl(commands, stdoutLines)
			err := uninstall(cli)
			Expect(err).NotTo(HaveOccurred(), "An error should not occur if the installation ID is not discoverable but it was forced")
		})
	})
})
