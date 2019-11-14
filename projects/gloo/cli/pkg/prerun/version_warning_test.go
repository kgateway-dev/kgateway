package prerun_test

import (
	"fmt"
	"strings"

	linkedversion "github.com/solo-io/gloo/pkg/version"
	version2 "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/versionutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
)

type testVersionGetter struct {
	versions []*version.ServerVersion
	err      error
}

func (t *testVersionGetter) Get() ([]*version.ServerVersion, error) {
	return t.versions, t.err
}

var _ version2.ServerVersion = &testVersionGetter{}

type testLogger struct {
	printedLines []string
}

func (t *testLogger) Printf(format string, args ...interface{}) {
	t.printedLines = append(t.printedLines, fmt.Sprintf(format, args...))
}

func (t *testLogger) Println(str string) {
	t.printedLines = append(t.printedLines, str)
}

func (t *testLogger) Sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

var _ prerun.Logger = &testLogger{}

var _ = Describe("version command", func() {
	var (
		binaryName = "glooctl-version-warn-unit-test-binary"
		namespace  = "test-namespace"
		v_20_12    = "0.20.12"
		v_20_13    = "0.20.13"
		v_21_0     = "0.21.0"
		v_1_0_0    = "1.0.0"

		buildContainerVersions = func(isEnterprise bool, containers []*version.Kubernetes_Container) []*version.ServerVersion {
			return []*version.ServerVersion{{
				Type:       version.GlooType_Gateway,
				Enterprise: isEnterprise,
				VersionType: &version.ServerVersion_Kubernetes{
					Kubernetes: &version.Kubernetes{
						Containers: containers,
						Namespace:  namespace,
					},
				},
			}}
		}

		err                 error
		versionGetter       *testVersionGetter
		logger              *testLogger
		expectedOutputLines []string
	)

	BeforeEach(func() {
		err = nil
		versionGetter = &testVersionGetter{}
		logger = &testLogger{}
		expectedOutputLines = []string{}

		// this may not be set in some contexts (like running through goland)
		// so explicitly set it to get predictable test behavior
		linkedversion.Version = v_20_12
	})

	AfterEach(func() {
		Expect(err).NotTo(HaveOccurred(), "No error should be returned from this prerun func")

		output := strings.Join(logger.printedLines, "\n")
		for _, line := range expectedOutputLines {
			Expect(output).To(ContainSubstring(line), "Output did not contain expected substring")
		}
	})

	It("should not warn when the versions match exactly", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_20_12,
			Name:     "test-name",
			Registry: "test-registry",
		}})

		err = prerun.WarnOnMismatch(binaryName, versionGetter, logger)
		Expect(logger.printedLines).To(BeEmpty(), "Should not warn when the versions match exactly")
	})

	It("should not warn when the versions differ only by patch version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_20_13,
			Name:     "test-name",
			Registry: "test-registry",
		}})

		err = prerun.WarnOnMismatch(binaryName, versionGetter, logger)
		Expect(logger.printedLines).To(BeEmpty(), "Should not warn when the versions differ only by patch version")
	})

	It("should warn when the versions differ by minor version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_21_0,
			Name:     "test-name",
			Registry: "test-registry",
		}})

		mismatches := []*prerun.ContainerVersion{{
			ContainerName: "test-name",
			Version: &versionutils.Version{
				Major: 0,
				Minor: 21,
				Patch: 0,
			},
		}}

		expectedOutputLines = []string{
			prerun.BuildVersionMismatchMessage(mismatches, "v"+linkedversion.Version, "minor"),
			prerun.BuildSuggestedUpgradeCommand(binaryName, mismatches),
		}

		err = prerun.WarnOnMismatch(binaryName, versionGetter, logger)
	})

	It("should warn when the versions differ by major version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_1_0_0,
			Name:     "test-name",
			Registry: "test-registry",
		}})

		mismatches := []*prerun.ContainerVersion{{
			ContainerName: "test-name",
			Version: &versionutils.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
		}}

		expectedOutputLines = []string{
			prerun.BuildVersionMismatchMessage(mismatches, "v"+linkedversion.Version, "major"),
			prerun.BuildSuggestedUpgradeCommand(binaryName, mismatches),
		}

		err = prerun.WarnOnMismatch(binaryName, versionGetter, logger)
	})
})
