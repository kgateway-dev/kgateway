package trafficpolicy

import (
	"testing"
	"time"

	faulthttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

//go:fix inline
func ptr32(i int32) *int32 { return new(i) }

//go:fix inline
func ptrU32(i uint32) *uint32 { return new(i) }

func TestFaultInjectionIREquals(t *testing.T) {
	delay100ms := &kgateway.FaultInjectionPolicy{
		Delay: &kgateway.FaultDelay{
			FixedDelay: metav1.Duration{Duration: 100 * time.Millisecond},
			Percentage: ptr32(50),
		},
	}
	delay200ms := &kgateway.FaultInjectionPolicy{
		Delay: &kgateway.FaultDelay{
			FixedDelay: metav1.Duration{Duration: 200 * time.Millisecond},
			Percentage: ptr32(50),
		},
	}

	tests := []struct {
		name string
		a, b *kgateway.FaultInjectionPolicy
		want bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name: "nil vs non-nil are not equal",
			a:    nil,
			b:    delay100ms,
			want: false,
		},
		{
			name: "same delay are equal",
			a:    delay100ms,
			b:    delay100ms,
			want: true,
		},
		{
			name: "different delays are not equal",
			a:    delay100ms,
			b:    delay200ms,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			aOut := &trafficPolicySpecIr{}
			constructFaultInjection(kgateway.TrafficPolicySpec{FaultInjection: tt.a}, aOut)

			bOut := &trafficPolicySpecIr{}
			constructFaultInjection(kgateway.TrafficPolicySpec{FaultInjection: tt.b}, bOut)

			a.Equal(tt.want, aOut.faultInjection.Equals(bOut.faultInjection))
		})
	}
}

func TestConstructFaultInjection(t *testing.T) {
	tests := []struct {
		name   string
		spec   kgateway.FaultInjectionPolicy
		verify func(t *testing.T, out *trafficPolicySpecIr)
	}{
		{
			name: "nil faultInjection field leaves IR nil",
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.Nil(t, out.faultInjection)
			},
		},
		{
			name: "delay only",
			spec: kgateway.FaultInjectionPolicy{
				Delay: &kgateway.FaultDelay{
					FixedDelay: metav1.Duration{Duration: 100 * time.Millisecond},
					Percentage: ptr32(75),
				},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault)
				assert.NotNil(t, out.faultInjection.httpFault.Delay)
				assert.Nil(t, out.faultInjection.httpFault.Abort)
				assert.Nil(t, out.faultInjection.httpFault.ResponseRateLimit)
				assert.EqualValues(t, 75, out.faultInjection.httpFault.Delay.GetPercentage().GetNumerator())
			},
		},
		{
			name: "abort with HTTP status",
			spec: kgateway.FaultInjectionPolicy{
				Abort: &kgateway.FaultAbort{
					HttpStatus: ptr32(503),
					Percentage: ptr32(100),
				},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault.Abort)
				assert.EqualValues(t, 503, out.faultInjection.httpFault.Abort.GetHttpStatus())
				assert.Nil(t, out.faultInjection.httpFault.Delay)
			},
		},
		{
			name: "abort with gRPC status",
			spec: kgateway.FaultInjectionPolicy{
				Abort: &kgateway.FaultAbort{
					GrpcStatus: ptr32(14), // UNAVAILABLE
					Percentage: ptr32(100),
				},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault.Abort)
				assert.EqualValues(t, 14, out.faultInjection.httpFault.Abort.GetGrpcStatus())
			},
		},
		{
			name: "response rate limit",
			spec: kgateway.FaultInjectionPolicy{
				ResponseRateLimit: &kgateway.FaultResponseRateLimit{
					KbitsPerSecond: 512,
					Percentage:     ptr32(100),
				},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault.ResponseRateLimit)
				assert.EqualValues(t, 512, out.faultInjection.httpFault.ResponseRateLimit.GetFixedLimit().GetLimitKbps())
			},
		},
		{
			name: "maxActiveFaults",
			spec: kgateway.FaultInjectionPolicy{
				Delay: &kgateway.FaultDelay{
					FixedDelay: metav1.Duration{Duration: 50 * time.Millisecond},
				},
				MaxActiveFaults: ptrU32(10),
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault.MaxActiveFaults)
				assert.EqualValues(t, 10, out.faultInjection.httpFault.MaxActiveFaults.GetValue())
			},
		},
		{
			name: "delay and abort combined",
			spec: kgateway.FaultInjectionPolicy{
				Delay: &kgateway.FaultDelay{
					FixedDelay: metav1.Duration{Duration: 200 * time.Millisecond},
					Percentage: ptr32(50),
				},
				Abort: &kgateway.FaultAbort{
					HttpStatus: ptr32(500),
					Percentage: ptr32(25),
				},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.NotNil(t, out.faultInjection.httpFault.Delay)
				assert.NotNil(t, out.faultInjection.httpFault.Abort)
				assert.EqualValues(t, 50, out.faultInjection.httpFault.Delay.GetPercentage().GetNumerator())
				assert.EqualValues(t, 500, out.faultInjection.httpFault.Abort.GetHttpStatus())
			},
		},
		{
			name: "disable produces non-nil IR with nil httpFault",
			spec: kgateway.FaultInjectionPolicy{
				Disable: &shared.PolicyDisable{},
			},
			verify: func(t *testing.T, out *trafficPolicySpecIr) {
				assert.NotNil(t, out.faultInjection)
				assert.Nil(t, out.faultInjection.httpFault)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &trafficPolicySpecIr{}
			var spec kgateway.TrafficPolicySpec
			if tt.name != "nil faultInjection field leaves IR nil" {
				spec.FaultInjection = &tt.spec
			}
			constructFaultInjection(spec, out)
			tt.verify(t, out)
		})
	}
}

func TestHandleFaultInjection_Disable(t *testing.T) {
	t.Run("nil IR adds no config", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		typedFilterConfig := ir.TypedFilterConfigMap{}
		p.handleFaultInjection("test-chain", &typedFilterConfig, nil)

		assert.Nil(t, typedFilterConfig[faultFilterName], "expected no typed config for fault filter")
	})

	t.Run("non-nil httpFault adds typed config", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		typedFilterConfig := ir.TypedFilterConfigMap{}
		fi := &faultInjectionIR{httpFault: &faulthttpv3.HTTPFault{
			Abort: &faulthttpv3.FaultAbort{
				ErrorType: &faulthttpv3.FaultAbort_HttpStatus{HttpStatus: 503},
			},
		}}
		p.handleFaultInjection("test-chain", &typedFilterConfig, fi)

		cfg := typedFilterConfig[faultFilterName]
		assert.NotNil(t, cfg, "expected typed config for fault filter")
		httpFault, ok := cfg.(*faulthttpv3.HTTPFault)
		assert.True(t, ok)
		assert.NotNil(t, httpFault.Abort)
	})

	t.Run("disable adds empty typed config to override parent", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		typedFilterConfig := ir.TypedFilterConfigMap{}
		fi := &faultInjectionIR{httpFault: nil}
		p.handleFaultInjection("test-chain", &typedFilterConfig, fi)

		cfg := typedFilterConfig[faultFilterName]
		assert.NotNil(t, cfg, "expected typed config for fault filter to override parent")
		httpFault, ok := cfg.(*faulthttpv3.HTTPFault)
		assert.True(t, ok, "expected HTTPFault type")
		assert.Nil(t, httpFault.Abort, "expected no abort in disable override")
		assert.Nil(t, httpFault.Delay, "expected no delay in disable override")
	})
}
