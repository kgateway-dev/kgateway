package stateful_session_test

import (
	envoyv3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	statefulsessionv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/stateful_session/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	cookiev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/cookie/v3"
	headerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/http/stateful_session/header/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/http/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/statefulsession"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/stateful_session"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/skv2/test/matchers"
	"google.golang.org/protobuf/types/known/durationpb"
)

var _ = Describe("stateful session", func() {
	Context("CookieBased", func() {
		It("works with minimal configuration", func() {
			name := "my-cookie"

			filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					StatefulSession: &statefulsession.StatefulSession{
						SessionState: &statefulsession.StatefulSession_CookieBased{
							CookieBased: &statefulsession.CookieBasedSessionState{
								Cookie: &statefulsession.CookieBasedSessionState_Cookie{
									Name: name,
								},
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			expectedStageFilter := expectedCookieFilter(name, "", nil, false)
			Expect(filters[0].Filter).To(matchers.MatchProto(expectedStageFilter.Filter))
		})

		It("works with all fields defined", func() {
			name := "my-cookie"
			path := "/not-the-default-path"
			d := &durationpb.Duration{
				Seconds: 3600,
			}
			strict := true

			filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					StatefulSession: &statefulsession.StatefulSession{
						SessionState: &statefulsession.StatefulSession_CookieBased{
							CookieBased: &statefulsession.CookieBasedSessionState{
								Cookie: &statefulsession.CookieBasedSessionState_Cookie{
									Name: name,
									Path: path,
									Ttl:  d,
								},
							},
						},
						Strict: strict,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			expectedStageFilter := expectedCookieFilter(name, path, d, strict)
			Expect(filters[0].Filter).To(matchers.MatchProto(expectedStageFilter.Filter))
		})

		DescribeTable("Bad configuration", func(statefulSession *statefulsession.StatefulSession, expectedErr error) {
			_, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					StatefulSession: statefulSession,
				},
			})

			Expect(eris.Is(err, expectedErr)).To(BeTrue())

			Expect(err).To(MatchError(expectedErr))
		},
			Entry("missing cookie name", &statefulsession.StatefulSession{
				SessionState: &statefulsession.StatefulSession_CookieBased{
					CookieBased: &statefulsession.CookieBasedSessionState{
						Cookie: &statefulsession.CookieBasedSessionState_Cookie{},
					},
				},
			},
				ErrNoCookieName,
			),
			Entry("missing cookie", &statefulsession.StatefulSession{
				SessionState: &statefulsession.StatefulSession_CookieBased{
					CookieBased: &statefulsession.CookieBasedSessionState{},
				},
			},
				ErrNoCookie,
			),
			Entry("missing cookie based Config", &statefulsession.StatefulSession{
				SessionState: &statefulsession.StatefulSession_CookieBased{},
			},
				ErrNoCookieBasedConfig,
			),
		)
	})

	Context("HeaderBased", func() {
		It("works with minimal config", func() {
			name := "my-header"

			filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
				Options: &v1.HttpListenerOptions{
					StatefulSession: &statefulsession.StatefulSession{
						SessionState: &statefulsession.StatefulSession_HeaderBased{
							HeaderBased: &statefulsession.HeaderBasedSessionState{
								HeaderName: name,
							},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			expectedStageFilter := expectedHeaderFilter(name, false)
			Expect(filters[0].Filter).To(matchers.MatchProto(expectedStageFilter.Filter))
		})
	})

	It("works with all fields defined", func() {
		name := "my-header"

		filters, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				StatefulSession: &statefulsession.StatefulSession{
					SessionState: &statefulsession.StatefulSession_HeaderBased{
						HeaderBased: &statefulsession.HeaderBasedSessionState{
							HeaderName: name,
						},
					},
					Strict: true,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		expectedStageFilter := expectedHeaderFilter(name, true)
		Expect(filters[0].Filter).To(matchers.MatchProto(expectedStageFilter.Filter))
	})

	DescribeTable("Bad configuration", func(statefulSession *statefulsession.StatefulSession, expectedErr error) {
		_, err := NewPlugin().HttpFilters(plugins.Params{}, &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				StatefulSession: statefulSession,
			},
		})

		Expect(eris.Is(err, expectedErr)).To(BeTrue())

		Expect(err).To(MatchError(expectedErr))
	},
		Entry("missing header name", &statefulsession.StatefulSession{
			SessionState: &statefulsession.StatefulSession_HeaderBased{
				HeaderBased: &statefulsession.HeaderBasedSessionState{},
			},
		},
			ErrNoHeaderName,
		),
		Entry("missing cookie based Config", &statefulsession.StatefulSession{
			SessionState: &statefulsession.StatefulSession_HeaderBased{},
		},
			ErrNoHeaderBasedConfig,
		),
	)
})

func expectedHeaderFilter(name string, strict bool) plugins.StagedHttpFilter {
	GinkgoHelper()

	// Create cookie based session state config
	headerBasedSessionStateConfig, err := utils.MessageToAny(
		&headerv3.HeaderBasedSessionState{
			Name: name,
		},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create stateful session config
	statefulSessionConfig := &statefulsessionv3.StatefulSession{
		SessionState: &envoyv3.TypedExtensionConfig{
			Name:        ExtensionTypeHeader,
			TypedConfig: headerBasedSessionStateConfig,
		},
	}

	if strict {
		statefulSessionConfig.Strict = true
	}

	statefulSessionMarshalled, err := utils.MessageToAny(statefulSessionConfig)

	Expect(err).NotTo(HaveOccurred())

	// Wrap it all up in a staged filter
	expectedStageFilter := plugins.StagedHttpFilter{
		Filter: &envoyhcm.HttpFilter{
			Name: ExtensionName,
			ConfigType: &envoyhcm.HttpFilter_TypedConfig{
				TypedConfig: statefulSessionMarshalled,
			},
		},

		Stage: plugins.HTTPFilterStage{
			RelativeTo: plugins.RouteStage,
			Weight:     0,
		},
	}

	return expectedStageFilter
}

func expectedCookieFilter(name, path string, d *durationpb.Duration, strict bool) plugins.StagedHttpFilter {
	GinkgoHelper()

	// Create cookie config
	cookie := &httpv3.Cookie{}

	Expect(name).NotTo(BeEmpty())
	cookie.Name = name

	if path != "" {
		cookie.Path = path
	}

	if d != nil {
		cookie.Ttl = d
	}

	// Create cookie based session state config
	cookieBasedSessionStateConfig, err := utils.MessageToAny(
		&cookiev3.CookieBasedSessionState{
			Cookie: cookie,
		},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create stateful session config
	statefulSessionConfig := &statefulsessionv3.StatefulSession{
		SessionState: &envoyv3.TypedExtensionConfig{
			Name:        ExtensionTypeCookie,
			TypedConfig: cookieBasedSessionStateConfig,
		},
	}

	if strict {
		statefulSessionConfig.Strict = true
	}

	statefulSessionMarshalled, err := utils.MessageToAny(statefulSessionConfig)

	Expect(err).NotTo(HaveOccurred())

	// Wrap it all up in a staged filter
	expectedStageFilter := plugins.StagedHttpFilter{
		Filter: &envoyhcm.HttpFilter{
			Name: ExtensionName,
			ConfigType: &envoyhcm.HttpFilter_TypedConfig{
				TypedConfig: statefulSessionMarshalled,
			},
		},

		Stage: plugins.HTTPFilterStage{
			RelativeTo: plugins.RouteStage,
			Weight:     0,
		},
	}

	return expectedStageFilter
}
