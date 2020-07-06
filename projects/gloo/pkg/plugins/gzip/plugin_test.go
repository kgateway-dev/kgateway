package gzip_test

import (
	envoycompressor "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoygzip "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/gzip/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/gzip"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Plugin", func() {
	It("copies the gzip config from the listener to the filter", func() {
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					MemoryLevel: &types.UInt32Value{
						Value: 10,
					},
					CompressionLevel:    v2.Gzip_CompressionLevel_SPEED,
					CompressionStrategy: v2.Gzip_HUFFMAN,
					WindowBits: &types.UInt32Value{
						Value: 10,
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: wellknown.Gzip,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoygzip.Gzip{
							MemoryLevel:         &wrappers.UInt32Value{Value: 10.000000},
							CompressionLevel:    envoygzip.Gzip_CompressionLevel_SPEED,
							CompressionStrategy: envoygzip.Gzip_HUFFMAN,
							WindowBits:          &wrappers.UInt32Value{Value: 10.000000},
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))

		By("with correct defaults")
		filters, err = NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: wellknown.Gzip,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoygzip.Gzip{
							CompressionLevel:    envoygzip.Gzip_CompressionLevel_DEFAULT,
							CompressionStrategy: envoygzip.Gzip_DEFAULT,
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

	It("copies the gzip config from the listener to the filter when deprecated fields are present", func() {
		By("when all deprecated fields are present")
		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					ContentLength: &types.UInt32Value{
						Value: 10,
					},
					ContentType:                []string{"type1", "type2"},
					DisableOnEtagHeader:        true,
					RemoveAcceptEncodingHeader: true,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: wellknown.Gzip,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoygzip.Gzip{
							Compressor: &envoycompressor.Compressor{
								ContentLength:              &wrappers.UInt32Value{Value: 10.000000},
								ContentType:                []string{"type1", "type2"},
								DisableOnEtagHeader:        true,
								RemoveAcceptEncodingHeader: true,
							},
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))

		By("when some deprecated fields are present")
		filters, err = NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					RemoveAcceptEncodingHeader: true,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(filters).To(Equal([]plugins.StagedHttpFilter{
			plugins.StagedHttpFilter{
				HttpFilter: &envoyhcm.HttpFilter{
					Name: wellknown.Gzip,
					ConfigType: &envoyhcm.HttpFilter_TypedConfig{
						TypedConfig: utils.MustMessageToAny(&envoygzip.Gzip{
							Compressor: &envoycompressor.Compressor{
								RemoveAcceptEncodingHeader: true,
							},
						}),
					},
				},
				Stage: plugins.FilterStage{
					RelativeTo: 8,
					Weight:     0,
				},
			},
		}))
	})

	It("errors copying the gzip config from the listener to the filter when an invalid enum value is used", func() {
		By("CompressionLevel")
		_, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					CompressionLevel: 3,
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid CompressionLevel 3"))

		By("CompressionStrategy")
		_, err = NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				Gzip: &v2.Gzip{
					CompressionStrategy: 4,
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid CompressionStrategy 4"))
	})
})
