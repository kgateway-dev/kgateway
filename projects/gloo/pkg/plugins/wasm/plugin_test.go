package wasm

import (
	"fmt"
	"os"

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
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
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
		Expect(os.Setenv(WasmEnabled, "1")).NotTo(HaveOccurred())
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
					Filters: []*wasm.WasmFilter{
						{
							Image: image,
						},
					},
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

	It("should not add filter if wasm env is not set", func() {
		Expect(os.Setenv(WasmEnabled, "")).NotTo(HaveOccurred())
		sha := "test-sha"
		image := "image"
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Filters: []*wasm.WasmFilter{
						{
							Image: image,
						},
					},
				},
			},
		}

		mockCache.EXPECT().Add(gomock.Any(), image).Return(digest.Digest(sha), nil)
		f, err := p.HttpFilters(plugins.Params{}, hl)
		Expect(err).NotTo(HaveOccurred())
		Expect(f).To(BeNil())
	})

	It("will return the proper config", func() {
		sha := "test-sha"
		image := "image"
		wasmFilter := &wasm.WasmFilter{
			Image:  image,
			Config: "test-config",
			Name:   "test",
			RootId: "test-root",
			VmType: wasm.WasmFilter_V8,
		}
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Filters: []*wasm.WasmFilter{wasmFilter},
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
		Expect(pc.Config.RootId).To(Equal(wasmFilter.RootId))
		Expect(pc.Config.Name).To(Equal(wasmFilter.Name))
		Expect(pc.Config.Configuration).To(Equal(wasmFilter.Config))
		Expect(pc.Config.VmConfig.VmId).To(Equal(VmId))
		Expect(pc.Config.VmConfig.Runtime).To(Equal(V8Runtime))
		remote := pc.Config.VmConfig.Code.GetRemote()
		Expect(remote).NotTo(BeNil())
		Expect(remote.Sha256).To(Equal(sha))
		Expect(remote.HttpUri.Uri).To(Equal(fmt.Sprintf("http://gloo/images/%s", sha)))
		Expect(remote.HttpUri.HttpUpstreamType).To(BeEquivalentTo(&core.HttpUri_Cluster{
			Cluster: WasmCacheCluster,
		}))
	})
})
