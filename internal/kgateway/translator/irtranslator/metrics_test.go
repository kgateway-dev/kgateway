package irtranslator_test

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	. "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
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

	// Generate sample data.
	tr.ComputeListener(ctx, nil, gw, lis, r)
	tr.ComputeListener(ctx, nil, gw, lis2, r)

	// Check if the metric for domains per listener is recorded correctly.
	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_routing_domains")

	gathered.AssertMetricsInclude("kgateway_routing_domains", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "namespace", Value: "default"},
				{Name: "gateway", Value: "gateway"},
				{Name: "port", Value: "80"},
			},
			Value: 3,
		}, &metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "namespace", Value: "default"},
				{Name: "gateway", Value: "gateway"},
				{Name: "port", Value: "443"},
			},
			Value: 1,
		},
	})
}
