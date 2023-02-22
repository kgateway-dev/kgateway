package translator_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("MergeSslConfig", func() {

	It("merges top-level SslConfig fields", func() {

		dst := &ssl.SslConfig{
			SniDomains:           []string{"dst"},
			VerifySubjectAltName: nil,
			OneWayTls: &wrappers.BoolValue{
				Value: false,
			},
		}

		src := &ssl.SslConfig{
			SniDomains:           []string{"src"},
			AlpnProtocols:        []string{"src"},
			VerifySubjectAltName: []string{"src"},
			OneWayTls:            nil,
		}

		expected := &ssl.SslConfig{
			// dst and src value, src should not override
			SniDomains: []string{"dst"},

			// missing dst value, src should override
			AlpnProtocols: []string{"src"},

			// nil dst value, src should override
			VerifySubjectAltName: []string{"src"},

			// nil src value, src should not override
			OneWayTls: &wrappers.BoolValue{
				Value: false,
			},
		}

		actual := translator.MergeSslConfig(dst, src)
		Expect(actual).To(Equal(expected))
	})

})
