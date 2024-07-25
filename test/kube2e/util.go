package kube2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/fsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/gomega/assertions"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kube2e/upgrade"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/stats"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
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
func GlooctlCheckEventuallyHealthy(offset int, namespace string, timeoutInterval string) {
	EventuallyWithOffset(offset, func() error {
		contextWithCancel, cancel := context.WithCancel(context.Background())
		defer cancel()
		opts := &options.Options{
			Metadata: core.Metadata{
				Namespace: namespace,
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

func UpdateDisableTransformationValidationSetting(ctx context.Context, shouldDisable bool, installNamespace string) {
	UpdateSettings(ctx, func(settings *v1.Settings) {
		Expect(settings.GetGateway().GetValidation()).NotTo(BeNil())
		settings.GetGateway().GetValidation().DisableTransformationValidation = &wrappers.BoolValue{Value: shouldDisable}
	}, installNamespace)
}

// enable/disable strict validation
func UpdateAlwaysAcceptSetting(ctx context.Context, alwaysAccept bool, installNamespace string) {
	UpdateSettings(ctx, func(settings *v1.Settings) {
		Expect(settings.GetGateway().GetValidation()).NotTo(BeNil())
		settings.GetGateway().GetValidation().AlwaysAccept = &wrappers.BoolValue{Value: alwaysAccept}
	}, installNamespace)
}

func UpdateAllowWarningsSetting(ctx context.Context, allowWarnings bool, installNamespace string) {
	UpdateSettings(ctx, func(settings *v1.Settings) {
		Expect(settings.GetGateway().GetValidation()).NotTo(BeNil())
		settings.GetGateway().GetValidation().AllowWarnings = &wrappers.BoolValue{Value: allowWarnings}
	}, installNamespace)
}

func UpdateRestEdsSetting(ctx context.Context, enableRestEds bool, installNamespace string) {
	UpdateSettings(ctx, func(settings *v1.Settings) {
		Expect(settings.GetGloo()).NotTo(BeNil())
		settings.GetGloo().EnableRestEds = &wrappers.BoolValue{Value: enableRestEds}
	}, installNamespace)
}

func UpdateReplaceInvalidRoutes(ctx context.Context, replaceInvalidRoutes bool, installNamespace string) {
	UpdateSettings(ctx, func(settings *v1.Settings) {
		Expect(settings.GetGloo().GetInvalidConfigPolicy()).NotTo(BeNil())
		settings.GetGloo().GetInvalidConfigPolicy().ReplaceInvalidRoutes = replaceInvalidRoutes
	}, installNamespace)
}

func UpdateSettings(ctx context.Context, updateSettings func(settings *v1.Settings), installNamespace string) {
	// when validation config changes, the validation server restarts -- give time for it to come up again.
	// without the wait, the validation webhook may temporarily fallback to it's failurePolicy, which is not
	// what we want to test.
	// TODO (samheilbron) We should avoid relying on time.Sleep in our tests as these tend to cause flakes
	waitForSettingsToPropagate := func() {
		time.Sleep(3 * time.Second)
	}
	UpdateSettingsWithPropagationDelay(updateSettings, waitForSettingsToPropagate, ctx, installNamespace)
}

func UpdateSettingsWithPropagationDelay(updateSettings func(settings *v1.Settings), waitForSettingsToPropagate func(), ctx context.Context, installNamespace string) {
	settingsClient := clienthelpers.MustSettingsClient(ctx)
	settings, err := settingsClient.Read(installNamespace, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())

	updateSettings(settings)

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())

	waitForSettingsToPropagate()
}

func ToFile(content string) string {
	fname, err := fsutils.ToTempFile(content)
	Expect(err).ToNot(HaveOccurred())

	return fname
}

// https://github.com/solo-io/gloo/issues/4043#issuecomment-772706604
// We should move tests away from using the testserver, and instead depend on EphemeralContainers.
// The default response changed in later kube versions, which caused this value to change.
// Ideally the test utilities used by Gloo are maintained in the Gloo repo, so I opted to move
// this constant here.
// This response is given by the testserver from the python2 SimpleHTTPServer
func TestServerHttpResponse() string {
	if runtime.GOARCH == "arm64" {
		return helper.SimpleHttpResponseArm
	} else {
		return helper.SimpleHttpResponse
	}
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
func GetTestHelper(ctx context.Context, namespace string) (*helper.SoloTestHelper, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	rootDir := filepath.Join(cwd, "../../..")
	return GetTestHelperForRootDir(ctx, rootDir, namespace)
}

func GetTestHelperForRootDir(ctx context.Context, rootDir, namespace string) (*helper.SoloTestHelper, error) {
	if useVersion := GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		return helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.ReleasedVersion = useVersion
			defaults.Verbose = true
			return defaults
		})
	} else {
		return helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = rootDir
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			defaults.Verbose = true
			return defaults
		})
	}
}
