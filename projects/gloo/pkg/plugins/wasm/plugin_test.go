package wasm

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
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
		hl := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Wasm: &wasm.PluginSource{
					Image:       "image",
					Config:      "test-config",
					FilterStage: "",
					Name:        "",
					RootId:      "test-root",
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

})
