package wasm

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/config"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	mock_cache "github.com/solo-io/gloo/projects/gloo/pkg/plugins/wasm/mocks"
	"github.com/solo-io/go-utils/errors"
)

var _ = Describe("wasm plugin", func() {
	var (
		p         *Plugin
		ctrl      *gomock.Controller
		mockCache *mock_cache.MockCache
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockCache = mock_cache.NewMockCache(ctrl)
		imageCache = mockCache
		p = NewPlugin()
	})

	It("should not add filter if wasm config is nil", func() {
		f, err := p.HttpFilters(plugins.Params{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will err if plugin cache returns an error", func() {
		image := "hello"
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Image: image,
				},
			},
		}

		fakeErr := errors.New("hello")
		mockCache.EXPECT().Add(gomock.Any(), image).Return(digest.Digest(""), fakeErr)
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).To(HaveOccurred())
		Expect(f).To(BeNil())
		Expect(err).To(Equal(fakeErr))
	})

	It("will return the proper config", func() {
		sha := "test-sha"
		image := "image"
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Image:       image,
					Config:      "test-config",
					Name:        "test",
					RootId:      "test-root",
				},
			},
		}

		mockCache.EXPECT().Add(gomock.Any(), image).Return(digest.Digest(sha), nil)
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(HaveLen(1))
		typedConfig := f[0].HttpFilter.GetConfig()
		var pc config.WasmService
		Expect(protoutils.UnmarshalStruct(typedConfig, &pc)).NotTo(HaveOccurred())
		Expect(pc.Config.RootId).To(Equal(hl.Options.Wasm.RootId))
		Expect(pc.Config.Name).To(Equal(hl.Options.Wasm.Name))
		Expect(pc.Config.Configuration).To(Equal(hl.Options.Wasm.Config))
	})

})
