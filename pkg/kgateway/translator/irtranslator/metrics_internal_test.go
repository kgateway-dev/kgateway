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
)

func TestClassifyErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "policy_not_found",
			err:  krtcollections.ErrPolicyNotFound,
			want: errTypeRefNotFound,
		},
		{
			name: "missing_reference_grant",
			err:  krtcollections.ErrMissingReferenceGrant,
			want: errTypeRefNotFound,
		},
		{
			name: "gateway_extension_not_found",
			err:  pluginutils.ErrGatewayExtensionNotFound,
			want: errTypeRefNotFound,
		},
		{
			name: "wrapped_gateway_extension_not_found",
			err:  fmt.Errorf("extauth: %w", fmt.Errorf("default/missing: %w", pluginutils.ErrGatewayExtensionNotFound)),
			want: errTypeRefNotFound,
		},
		{
			name: "invalid_matcher",
			err:  ErrInvalidMatcher,
			want: errTypeInvalidCfg,
		},
		{
			name: "invalid_route",
			err:  ErrInvalidRoute,
			want: errTypeInvalidCfg,
		},
		{
			name: "unknown_backend_kind",
			err:  krtcollections.ErrUnknownBackendKind,
			want: errTypeInvalidCfg,
		},
		{
			name: "extension_type_error_via_errors_as",
			err:  pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeExtAuth),
			want: errTypeInvalidCfg,
		},
		{
			name: "wrapped_extension_type_error",
			err:  fmt.Errorf("extauth: %w", pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeExtAuth)),
			want: errTypeInvalidCfg,
		},
		{
			name: "bare_unrecognised_error",
			err:  errors.New("some random error"),
			want: errTypeUnknown,
		},
		{
			name: "join_first_leaf_wins",
			err:  errors.Join(pluginutils.ErrGatewayExtensionNotFound, ErrInvalidMatcher),
			want: errTypeRefNotFound,
		},
		{
			name: "join_first_leaf_wins_reversed",
			err:  errors.Join(ErrInvalidMatcher, pluginutils.ErrGatewayExtensionNotFound),
			want: errTypeInvalidCfg,
		},
		{
			name: "join_all_unknown",
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
			Obj: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw-a",
					Namespace: "ns-a",
				},
			},
		},
	}

	incRouteReplacementMetric(gw, krtcollections.ErrPolicyNotFound)
	incRouteReplacementMetric(gw, ErrInvalidMatcher)
	incRouteReplacementMetric(gw, errors.New("uncategorised"))

	gathered := metricstest.MustGatherMetricsContext(ctx, t, "kgateway_routing_replacements_total")
	gathered.AssertMetricsInclude("kgateway_routing_replacements_total", []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway_namespace", Value: "ns-a"},
				{Name: "gateway", Value: "gw-a"},
				{Name: "error_type", Value: errTypeRefNotFound},
			},
			Value: 1,
		},
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway_namespace", Value: "ns-a"},
				{Name: "gateway", Value: "gw-a"},
				{Name: "error_type", Value: errTypeInvalidCfg},
			},
			Value: 1,
		},
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "gateway_namespace", Value: "ns-a"},
				{Name: "gateway", Value: "gw-a"},
				{Name: "error_type", Value: errTypeUnknown},
			},
			Value: 1,
		},
	})
}
