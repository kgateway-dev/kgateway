package validation_test

import (
	"context"

	"github.com/solo-io/gloo/test/samples"

	validationgrpc "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/validation"

	"github.com/golang/mock/gomock"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul"

	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	. "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
)

var _ = Describe("Validation Server", func() {
	var (
		ctrl              *gomock.Controller
		settings          *v1.Settings
		translator        Translator
		params            plugins.Params
		registeredPlugins []plugins.Plugin
	)

	BeforeEach(func() {

		ctrl = gomock.NewController(T)

		settings = &v1.Settings{}
		memoryClientFactory := &factory.MemoryResourceClientFactory{
			Cache: memory.NewInMemoryResourceCache(),
		}
		opts := bootstrap.Opts{
			Settings:      settings,
			Secrets:       memoryClientFactory,
			Upstreams:     memoryClientFactory,
			ConsulWatcher: consul.NewMockConsulWatcher(ctrl), // just needed to activate the consul plugin
		}
		registeredPlugins = registry.Plugins(opts)

		params = plugins.Params{
			Ctx:      context.Background(),
			Snapshot: samples.SimpleGlooSnapshot(),
		}
	})

	JustBeforeEach(func() {
		translator = NewTranslator(sslutils.NewSslConfigTranslator(), settings, registeredPlugins...)
	})

	It("validates the requested proxy", func() {
		proxy := params.Snapshot.Proxies[0]
		s := NewValidator(translator)
		_ = s.Sync(context.TODO(), params.Snapshot)
		rpt, err := s.ValidateProxy(context.TODO(), &validationgrpc.ProxyValidationServiceRequest{Proxy: proxy})
		Expect(err).NotTo(HaveOccurred())
		Expect(rpt).To(Equal(&validationgrpc.ProxyValidationServiceResponse{ProxyReport: validation.MakeReport(proxy)}))
	})
})
