package prerun_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	linkedversion "github.com/solo-io/gloo/pkg/version"
	version2 "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/versionutils"
)

type testVersionGetter struct {
	versions          []*version.ServerVersion
	kubernetesVersion *version.KubernetesClusterVersion
	err               error
}

func (t *testVersionGetter) Get(ctx context.Context) ([]*version.ServerVersion, error) {
	return t.versions, t.err
}

func (t *testVersionGetter) GetClusterVersion() (*version.KubernetesClusterVersion, error) {
	return t.kubernetesVersion, t.err
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
		binaryName   = "glooctl-version-warn-unit-test-binary"
		namespace    = "test-namespace"
		otherPodName = "other-pod"

		v_20_12 = "0.20.12"
		v_20_13 = "0.20.13"
		v_21_0  = "0.21.0"
		v_1_0_0 = "1.0.0"
		ctx     context.Context
		cancel  context.CancelFunc

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
		ctx, cancel = context.WithCancel(context.Background())

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
		cancel()
	})

	It("should not warn when the versions match exactly", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_20_12,
			Name:     "gloo",
			Registry: "test-registry",
			OssTag:   v_20_12,
		}})

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
		Expect(logger.printedLines).To(BeEmpty(), "Should not warn when the versions match exactly")
	})

	It("should not warn when the versions differ only by patch version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_20_13,
			Name:     "gloo",
			Registry: "test-registry",
			OssTag:   v_20_13,
		}})

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
		Expect(logger.printedLines).To(BeEmpty(), "Should not warn when the versions differ only by patch version")
	})

	It("should warn when the versions differ on the gloo-ee pod", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_21_0,
			Name:     "gloo-ee",
			Registry: "test-registry",
			OssTag:   v_21_0,
		}})

		mismatches := []*versionutils.Version{{
			Major: 0,
			Minor: 21,
			Patch: 0,
		}}

		expectedOutputLines = []string{
			prerun.BuildSuggestedUpgradeCommand(binaryName, mismatches),
		}

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
	})
	It("should warn when the versions differ by minor version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_21_0,
			Name:     "gloo",
			Registry: "test-registry",
			OssTag:   v_21_0,
		}})

		mismatches := []*versionutils.Version{{
			Major: 0,
			Minor: 21,
			Patch: 0,
		}}

		expectedOutputLines = []string{
			prerun.BuildSuggestedUpgradeCommand(binaryName, mismatches),
		}

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
	})

	It("should warn when the versions differ by major version", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{{
			Tag:      v_1_0_0,
			Name:     "gloo",
			Registry: "test-registry",
			OssTag:   v_1_0_0,
		}})

		mismatches := []*versionutils.Version{{
			Major: 1,
			Minor: 0,
			Patch: 0,
		}}

		expectedOutputLines = []string{
			prerun.BuildSuggestedUpgradeCommand(binaryName, mismatches),
		}

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
	})

	It("should ignore containers other than the one we specifically look for", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{
			{
				Tag:      v_20_12,
				Name:     "gloo",
				Registry: "test-registry",
				OssTag:   v_20_12,
			},
			{
				Tag:      v_1_0_0,
				Name:     otherPodName,
				Registry: "test-registry",
				OssTag:   v_1_0_0,
			},
		})

		err = prerun.WarnOnMismatch(ctx, binaryName, versionGetter, logger)
		Expect(logger.printedLines).To(BeEmpty(), "Should not warn when the versions match exactly")
	})

	It("should return proper oss tag based on gloo container", func() {
		versionGetter.versions = buildContainerVersions(false, []*version.Kubernetes_Container{
			// We define a version, that includes a label
			{
				Tag:      "v1.0.0-ci",
				Name:     "gloo",
				Registry: "test-registry",
				OssTag:   "1.0.0-ci",
			},
			// We define a version, that includes a label AND non-zero labelVersion
			{
				Tag:      "v1.0.0-ci1",
				Name:     "gloo",
				Registry: "test-registry",
				OssTag:   "1.0.0-ci1",
			},
		})

		versions, err := prerun.GetOpenSourceVersions(versionGetter.versions)
		Expect(err).NotTo(HaveOccurred())
		Expect(versions).To(And(
			HaveLen(2),
			ContainElement(&versionutils.Version{
				Major:        1,
				Minor:        0,
				Patch:        0,
				Label:        "ci",
				LabelVersion: 0,
			}),
			ContainElement(&versionutils.Version{
				Major:        1,
				Minor:        0,
				Patch:        0,
				Label:        "ci",
				LabelVersion: 1,
			}),
		))
		// This test demonstrates a potential gotcha when working with versions that supply a label
		// but do not supply a labelVersion. When invoking the Stringer, the entire label is removed
		Expect(versions[0].String()).To(Equal("v1.0.0"))

		Expect(versions[1].String()).To(Equal("v1.0.0-ci1"))
	})
})
