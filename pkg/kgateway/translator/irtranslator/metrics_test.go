package irtranslator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestDomainsPerListenerMetric(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &Translator{}

	gw := ir.GatewayIR{
		SourceObject: &ir.Gateway{
			Listeners: []ir.Listener{
				{Listener: gwv1.Listener{
					Name:     "listener1",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
				}},
				{Listener: gwv1.Listener{
					Name:     "listener2",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
				}},
			},
			Obj: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway",
					Namespace: "default",
				},
			},
		},
	}

	lis := ir.ListenerIR{
		Name:     "listener1",
		BindPort: 80,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			Vhosts: []*ir.VirtualHost{
				{
					Name:     "example.com",
					Hostname: "example.com",
				},
				{
					Name:     "example.org",
					Hostname: "example.org",
				},
			},
		}, {
			Vhosts: []*ir.VirtualHost{
				{
					Name:     "example.net",
					Hostname: "example.net",
				},
				{
					Name:     "example.org",
					Hostname: "example.org",
				},
			},
		}},
	}

	lis2 := ir.ListenerIR{
		Name:     "listener2",
		BindPort: 443,
		HttpFilterChain: []ir.HttpFilterChainIR{{
			Vhosts: []*ir.VirtualHost{
				{
					Name:     "example.io",
					Hostname: "example.io",
				},
			},
		}},
	}

	rm := reports.NewReportMap()
	r := reports.NewReporter(&rm)

	tr.ComputeListener(ctx, nil, gw, lis, r)
	tr.ComputeListener(ctx, nil, gw, lis2, r)

	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_routing_domains")

	gathered.AssertMetricsInclude("kgateway_routing_domains", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: namespaceLabel, Value: "default"},
				{Name: gatewayLabel, Value: "gateway"},
				{Name: portLabel, Value: "80"},
			},
			Value: 3,
		}, &metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: namespaceLabel, Value: "default"},
				{Name: gatewayLabel, Value: "gateway"},
				{Name: portLabel, Value: "443"},
			},
			Value: 1,
		},
	})
}

func TestClassifyErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil classifies as unknown",
			err:  nil,
			want: errTypeUnknown,
		},
		{
			name: "ErrPolicyNotFound classifies as ref_not_found",
			err:  krtcollections.ErrPolicyNotFound,
			want: errTypeRefNotFound,
		},
		{
			name: "ErrMissingReferenceGrant classifies as ref_not_found",
			err:  krtcollections.ErrMissingReferenceGrant,
			want: errTypeRefNotFound,
		},
		{
			name: "ErrGatewayExtensionNotFound classifies as ref_not_found",
			err:  pluginutils.ErrGatewayExtensionNotFound,
			want: errTypeRefNotFound,
		},
		{
			name: "wrapped ErrGatewayExtensionNotFound still classifies as ref_not_found",
			err:  fmt.Errorf("extauth: %w", fmt.Errorf("default/missing: %w", pluginutils.ErrGatewayExtensionNotFound)),
			want: errTypeRefNotFound,
		},
		{
			name: "ErrInvalidMatcher classifies as invalid_config",
			err:  ErrInvalidMatcher,
			want: errTypeInvalidCfg,
		},
		{
			name: "ErrInvalidRoute classifies as invalid_config",
			err:  ErrInvalidRoute,
			want: errTypeInvalidCfg,
		},
		{
			name: "ErrUnknownBackendKind classifies as invalid_config",
			err:  krtcollections.ErrUnknownBackendKind,
			want: errTypeInvalidCfg,
		},
		{
			name: "ExtensionTypeError classifies as invalid_config via errors.As",
			err:  pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeExtAuth),
			want: errTypeInvalidCfg,
		},
		{
			name: "wrapped ExtensionTypeError classifies as invalid_config",
			err:  fmt.Errorf("extauth: %w", pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeExtAuth)),
			want: errTypeInvalidCfg,
		},
		{
			name: "bare unrecognised error classifies as unknown",
			err:  errors.New("some random error"),
			want: errTypeUnknown,
		},
		{
			name: "first matching leaf wins on errors.Join",
			err:  errors.Join(pluginutils.ErrGatewayExtensionNotFound, ErrInvalidMatcher),
			want: errTypeRefNotFound,
		},
		{
			name: "first matching leaf wins on errors.Join (reverse order)",
			err:  errors.Join(ErrInvalidMatcher, pluginutils.ErrGatewayExtensionNotFound),
			want: errTypeInvalidCfg,
		},
		{
			name: "join of all-unknown leaves classifies as unknown",
			err:  errors.Join(errors.New("a"), errors.New("b")),
			want: errTypeUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyErr(tc.err))
		})
	}
}

func TestIncRouteReplacementLabels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gw := ir.GatewayIR{
		SourceObject: &ir.Gateway{
			ObjectSource: ir.ObjectSource{
				Name:      "gw-a",
				Namespace: "ns-a",
			},
		},
	}

	incRouteReplacementMetric(gw, krtcollections.ErrPolicyNotFound)
	incRouteReplacementMetric(gw, ErrInvalidMatcher)
	incRouteReplacementMetric(gw, errors.New("uncategorised"))

	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_routing_replacements_total")
	gathered.AssertMetrics("kgateway_routing_replacements_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: gatewayNamespaceLabel, Value: "ns-a"},
				{Name: gatewayLabel, Value: "gw-a"},
				{Name: errorTypeLabel, Value: errTypeRefNotFound},
			},
			Value: 1,
		},
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: gatewayNamespaceLabel, Value: "ns-a"},
				{Name: gatewayLabel, Value: "gw-a"},
				{Name: errorTypeLabel, Value: errTypeInvalidCfg},
			},
			Value: 1,
		},
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: gatewayNamespaceLabel, Value: "ns-a"},
				{Name: gatewayLabel, Value: "gw-a"},
				{Name: errorTypeLabel, Value: errTypeUnknown},
			},
			Value: 1,
		},
	})
}
