package translator_test

import (
	"context"
	"fmt"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"strings"

	"github.com/solo-io/go-utils/contextutils"

	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/gloo/test/ginkgo/labels"

	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/mock/gomock"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	mock_consul "github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul/mocks"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// benchmarkConfig allows configuration for benchmarking tests to be reused for similar cases
// This struct can be factored out to an accessible location should additional benchmarking suites be added
type benchmarkConfig struct {
	iterations        int                   // the number of iterations to attempt for a particular entry
	maxDur            time.Duration         // the maximum time to spend on a particular entry even if not all iterations are complete
	benchmarkMatchers []types.GomegaMatcher // matchers representing the assertions we wish to make for a particular entry
}

// Tests are run as part of the "Nightly" action in a GHA using the default Linux runner
// More info on that machine can be found here: https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners#supported-runners-and-hardware-resources
// When developing new tests, users should manually run that action in order to test performance under the same parameters
// Results can then be found in the logs for that instance of the action
var _ = Describe("Translation - Benchmarking Tests", decorators.Performance, Label(labels.Performance), func() {
	var (
		ctrl       *gomock.Controller
		settings   *v1.Settings
		translator Translator
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(T)

		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:  settings,
			Secrets:   memoryClientFactory,
			Upstreams: memoryClientFactory,
			Consul: bootstrap.Consul{
				ConsulWatcher: mock_consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
			},
		}
		registeredPlugins := registry.Plugins(opts)
		pluginRegistry := registry.NewPluginRegistry(registeredPlugins)

		translator = NewTranslatorWithHasher(glooutils.NewSslConfigTranslator(), settings, pluginRegistry, EnvoyCacheResourcesListToFnvHash)
	})

	// The Benchmark table takes entries consisting of an ApiSnapshot, benchmarkConfig, and labels
	// We measure the duration of the translation of the snapshot, benchmarking according to the benchmarkConfig
	// Labels are used to add context to the entry description
	DescribeTable("Benchmark table",
		func(snapBuilder *gloohelpers.ScaledSnapshotBuilder, config benchmarkConfig, labels ...string) {
			var (
				apiSnap *v1snap.ApiSnapshot
				proxy   *v1.Proxy

				snap   cache.Snapshot
				errs   reporter.ResourceReports
				report *validation.ProxyReport

				tooFastWarningCount int
			)

			apiSnap = snapBuilder.Build()

			params := plugins.Params{
				Ctx:      context.Background(),
				Snapshot: apiSnap,
			}

			Expect(apiSnap.Proxies).NotTo(BeEmpty())
			proxy = apiSnap.Proxies[0]

			desc := generateDesc(snapBuilder, config, labels...)

			experiment := gmeasure.NewExperiment(fmt.Sprintf("Experiment - %s", desc))

			AddReportEntry(experiment.Name, experiment)

			experiment.Sample(func(idx int) {

				// Time translation
				res, ignore, err := gloohelpers.MeasureIgnore0ns(func() {
					snap, errs, report = translator.Translate(params, proxy)
				})
				Expect(err).NotTo(HaveOccurred())

				if idx == 0 {
					// Assert expected results on the first sample
					Expect(errs.Validate()).NotTo(HaveOccurred())
					Expect(snap).NotTo(BeNil())
					Expect(report).To(Equal(validationutils.MakeReport(proxy)))
				}

				if ignore {
					tooFastWarningCount++
					return
				}

				// Benchmark total time spent
				// If desired, a field can be added to benchmarkConfig to allow benchmarking according to User and/or
				// System time
				experiment.RecordDuration(desc, res.Total)
			}, gmeasure.SamplingConfig{N: config.iterations, Duration: config.maxDur})

			if tooFastWarningCount > 0 {
				logger := contextutils.LoggerFrom(params.Ctx)
				logger.Warnf("entry %s registered %d 0ns measurements; consider increasing the scale of the proxy being tested for more accurate results", desc, tooFastWarningCount)
			}

			durations := experiment.Get(desc).Durations

			Expect(durations).Should(And(config.benchmarkMatchers...))
		},
		generateDesc, // generate descriptions for table entries with nil descriptions
		Entry("basic", gloohelpers.NewInjectedSnapshotBuilder(basicSnap), basicConfig),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(10).WithEndpointCount(1), basicConfig, "upstream scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(1000).WithEndpointCount(1), oneKUpstreamsConfig, "upstream scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(1).WithEndpointCount(10), basicConfig, "endpoint scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(1).WithEndpointCount(1000), basicConfig, "endpoint scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(10).WithEndpointCount(10), basicConfig, "endpoint scale", "upstream scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(10).WithEndpointCount(1).
			WithUpstreamBuilder(consistentSniUsBuilder), basicConfig, "consistent SNI", "upstream scale"),
		Entry(nil, gloohelpers.NewScaledSnapshotBuilder().WithUpstreamCount(10).WithEndpointCount(1).
			WithUpstreamBuilder(uniqueSniUsBuilder), basicConfig, "unique SNI", "upstream scale"),
	)
})

func generateDesc(b *gloohelpers.ScaledSnapshotBuilder, _ benchmarkConfig, labels ...string) string {
	labelPrefix := ""
	if len(labels) > 0 {
		labelPrefix = fmt.Sprintf("(%s) ", strings.Join(labels, ", "))
	}

	if b.HasInjectedSnapshot() {
		return fmt.Sprintf("%sinjected snapshot", labelPrefix)
	}

	// If/when additional Snapshot fields are included in testing, the description should be updated accordingly
	return fmt.Sprintf("%s%d endpoint(s), %d upstream(s)", labelPrefix, b.EndpointCount(), b.UpstreamCount())
}

// Test assets: Add blocks for logical groupings of tests, including:
// - in-line snapshot definitions for tests that require granularly-configured/heterogeneous resources (ie testing a particular field or feature)
// - benchmarkConfigs for particular groups of use cases depending on processing time requirements/expectations

/* Basic Tests */
var basicSnap = &v1snap.ApiSnapshot{
	Proxies: []*v1.Proxy{
		{
			Listeners: []*v1.Listener{
				gloohelpers.HttpListener(1),
			},
		},
	},
	Endpoints: []*v1.Endpoint{gloohelpers.Endpoint(0)},
	Upstreams: []*v1.Upstream{gloohelpers.Upstream(0)},
}

var basicConfig = benchmarkConfig{
	iterations: 1000,
	maxDur:     10 * time.Second,
	benchmarkMatchers: []types.GomegaMatcher{
		matchers.HaveMedianLessThan(50 * time.Millisecond),
		matchers.HavePercentileLessThan(90, 100*time.Millisecond),
	},
}

/* 1k Upstreams Scale Test */
var oneKUpstreamsConfig = benchmarkConfig{
	iterations: 100,
	maxDur:     30 * time.Second,
	benchmarkMatchers: []types.GomegaMatcher{
		matchers.HaveMedianLessThan(time.Second),
		matchers.HavePercentileLessThan(90, 2*time.Second),
	},
}

/* Upstream SNI Test */
var (
	consistentSniUsBuilder = gloohelpers.NewUpstreamBuilder().WithConsistentSni()
	uniqueSniUsBuilder     = gloohelpers.NewUpstreamBuilder().WithUniqueSni()
)
