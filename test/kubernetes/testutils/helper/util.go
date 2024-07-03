package helper

import (
	"context"
	"os"
	"runtime"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/test/gomega/assertions"
	"github.com/solo-io/gloo/test/kube2e/upgrade"
	e2edefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/stats"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap/zapcore"
)

const (
	// UniqueTestResourceLabel can be assigned to the resources used by kube2e tests
	// This unique label per test run ensures that the generated snapshot is different on subsequent runs
	// We have previously seen flakes where a resource is deleted and re-created with the same hash and thus
	// the emitter can miss the update
	UniqueTestResourceLabel = "gloo-kube2e-test-id"
)

func GetHttpEchoImage() string {
	httpEchoImage := "hashicorp/http-echo"
	if runtime.GOARCH == "arm64" {
		httpEchoImage = "gcr.io/solo-test-236622/http-echo"
	}
	return httpEchoImage
}

// GlooctlCheckEventuallyHealthy will run up until proved timeoutInterval or until gloo is reported as healthy
func GlooctlCheckEventuallyHealthy(offset int, testHelper *SoloTestHelper, timeoutInterval string) {
	EventuallyWithOffset(offset, func() error {
		contextWithCancel, cancel := context.WithCancel(context.Background())
		defer cancel()
		opts := &options.Options{
			Metadata: core.Metadata{
				Namespace: testHelper.InstallNamespace,
			},
			Top: options.Top{
				Ctx: contextWithCancel,
			},
		}
		err := check.CheckResources(contextWithCancel, printers.P{}, opts)
		if err != nil {
			return errors.Wrap(err, "glooctl check detected a problem with the installation")
		}
		return nil
	}, timeoutInterval, "5s").Should(BeNil())
}

func EventuallyReachesConsistentState(installNamespace string) {
	// We port-forward the Gloo deployment stats port to inspect the metrics and log settings
	glooStatsForwardConfig := assertions.StatsPortFwd{
		ResourceName:      "deployment/gloo",
		ResourceNamespace: installNamespace,
		LocalPort:         stats.DefaultPort,
		TargetPort:        stats.DefaultPort,
	}

	// Gloo components are configured to log to the Info level by default
	logLevelAssertion := assertions.LogLevelAssertion(zapcore.InfoLevel)

	// The emitter at some point should stabilize and not continue to increase the number of snapshots produced
	// We choose 4 here as a bit of a magic number, but we feel comfortable that if 4 consecutive polls of the metrics
	// endpoint returns that same value, then we have stabilized
	identicalResultInARow := 4
	emitterMetricAssertion, _ := assertions.IntStatisticReachesConsistentValueAssertion("api_gloosnapshot_gloo_solo_io_emitter_snap_out", identicalResultInARow)

	ginkgo.By("Gloo eventually reaches a consistent state")
	offset := 1 // This method is called directly from a TestSuite
	assertions.EventuallyWithOffsetStatisticsMatchAssertions(offset, glooStatsForwardConfig,
		logLevelAssertion.WithOffset(offset),
		emitterMetricAssertion.WithOffset(offset),
	)
}

// This response is given by the nginx pod defined in test/kubernetes/e2e/defaults/
func TestServerHttpResponse() string {
	return e2edefaults.NginxResponse
}

// For nightly runs, we want to install a released version rather than using a locally built chart
// To do this, set the environment variable RELEASED_VERSION with either a version name or "LATEST" to get the last release
func GetTestReleasedVersion(ctx context.Context, repoName string) string {
	releasedVersion := os.Getenv(testutils.ReleasedVersion)

	if releasedVersion == "" {
		// In the case where the released version is empty, we return an empty string
		// The function which consumes this value will then use the locally built chart
		return releasedVersion
	}

	if releasedVersion == "LATEST" {
		_, current, err := upgrade.GetUpgradeVersions(ctx, repoName)
		Expect(err).NotTo(HaveOccurred())
		return current.String()
	}

	// Assume that releasedVersion is a valid version, for a previously released version of Gloo Edge
	return releasedVersion
}

func GetTestHelperForRootDir(ctx context.Context, rootDir, namespace string) (*SoloTestHelper, error) {
	if useVersion := GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		return NewSoloTestHelper(func(defaults TestConfig) TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.ReleasedVersion = useVersion
			defaults.Verbose = true
			return defaults
		})
	} else {
		return NewSoloTestHelper(func(defaults TestConfig) TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
	}
}
