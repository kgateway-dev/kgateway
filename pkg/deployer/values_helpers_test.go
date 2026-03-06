package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestComponentLogLevelsToString(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]string
		want    string
		wantErr error
	}{
		{
			name:    "empty map should convert to empty string",
			input:   map[string]string{},
			want:    "",
			wantErr: nil,
		},
		{
			name:    "empty key should throw error",
			input:   map[string]string{"": "val"},
			want:    "",
			wantErr: ComponentLogLevelEmptyError("", "val"),
		},
		{
			name:    "empty value should throw error",
			input:   map[string]string{"key": ""},
			want:    "",
			wantErr: ComponentLogLevelEmptyError("key", ""),
		},
		{
			name: "should sort keys",
			input: map[string]string{
				"bbb": "val1",
				"cat": "val2",
				"a":   "val3",
			},
			want:    "a:val3,bbb:val1,cat:val2",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComponentLogLevelsToString(tt.input)
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSetLoadBalancerIPFromGateway(t *testing.T) {
	tests := []struct {
		name        string
		addresses   []gwv1.GatewaySpecAddress
		serviceType *string
		wantIP      *string
		wantErr     error
	}{
		{
			name: "single valid IPv4 address with LoadBalancer service",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      new("203.0.113.10"),
			wantErr:     nil,
		},
		{
			name: "single valid IPv6 address with LoadBalancer service",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "2001:db8::1"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      new("2001:db8::1"),
			wantErr:     nil,
		},
		{
			name: "nil address type defaults to IPAddressType",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: nil, Value: "192.0.2.1"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      new("192.0.2.1"),
			wantErr:     nil,
		},
		{
			name:        "empty addresses array with LoadBalancer service",
			addresses:   []gwv1.GatewaySpecAddress{},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     nil,
		},
		{
			name: "multiple valid IP addresses returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.11"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrMultipleAddresses,
		},
		{
			name: "multiple addresses with mixed types returns ip address",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.HostnameAddressType), Value: "example.com"},
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      new("203.0.113.10"),
			wantErr:     nil,
		},
		{
			name: "single hostname address returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.HostnameAddressType), Value: "example.com"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrNoValidIPAddress,
		},
		{
			name: "single invalid IP address returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "not-an-ip"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrNoValidIPAddress,
		},
		{
			name: "single invalid IP address format returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "256.256.256.256"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrNoValidIPAddress,
		},
		{
			name: "nil type with valid IP returns IP",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: nil, Value: "203.0.113.10"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      new("203.0.113.10"),
			wantErr:     nil,
		},
		{
			name: "nil type with invalid IP returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: nil, Value: "invalid"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrNoValidIPAddress,
		},
		{
			name: "three addresses returns error",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.11"},
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.12"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeLoadBalancer)),
			wantIP:      nil,
			wantErr:     ErrMultipleAddresses,
		},
		{
			name: "valid IP with ClusterIP service does not set IP",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
			},
			serviceType: ptr.To(string(corev1.ServiceTypeClusterIP)),
			wantIP:      nil,
			wantErr:     nil,
		},
		{
			name: "valid IP with nil service type does not set IP",
			addresses: []gwv1.GatewaySpecAddress{
				{Type: ptr.To(gwv1.IPAddressType), Value: "203.0.113.10"},
			},
			serviceType: nil,
			wantIP:      nil,
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					Addresses: tt.addresses,
				},
			}

			svc := &HelmService{
				Type: tt.serviceType,
			}

			err := SetLoadBalancerIPFromGateway(gw, svc)
			if tt.wantErr != nil {
				assert.Error(t, err, "expected error")
				assert.ErrorIs(t, err, tt.wantErr, "error type mismatch")
				assert.Nil(t, svc.LoadBalancerIP, "expected nil IP when error occurs")
			} else {
				assert.NoError(t, err, "unexpected error")
				if tt.wantIP == nil {
					assert.Nil(t, svc.LoadBalancerIP, "expected nil but got %v", svc.LoadBalancerIP)
				} else {
					assert.NotNil(t, svc.LoadBalancerIP, "expected non-nil IP")
					assert.Equal(t, *tt.wantIP, *svc.LoadBalancerIP, "IP address mismatch")
				}
			}
		})
	}
}

func TestGetServiceValues(t *testing.T) {
	lbType := corev1.ServiceTypeLoadBalancer

	tests := []struct {
		name  string
		input *kgateway.Service
		want  *HelmService
	}{
		{
			name:  "nil service config returns empty HelmService",
			input: nil,
			want:  &HelmService{},
		},
		{
			name:  "empty service config returns empty HelmService",
			input: &kgateway.Service{},
			want:  &HelmService{},
		},
		{
			name: "fully populated service config",
			input: &kgateway.Service{
				Type:                     &lbType,
				ClusterIP:                new("10.0.0.1"),
				ExtraLabels:              map[string]string{"env": "test"},
				ExtraAnnotations:         map[string]string{"note": "value"},
				ExternalTrafficPolicy:    new("Local"),
				LoadBalancerClass:        new("service.k8s.aws/nlb"),
				LoadBalancerSourceRanges: []string{"10.0.0.0/8", "192.168.0.0/16"},
			},
			want: &HelmService{
				Type:                     new("LoadBalancer"),
				ClusterIP:                new("10.0.0.1"),
				ExtraLabels:              map[string]string{"env": "test"},
				ExtraAnnotations:         map[string]string{"note": "value"},
				ExternalTrafficPolicy:    new("Local"),
				LoadBalancerClass:        new("service.k8s.aws/nlb"),
				LoadBalancerSourceRanges: []string{"10.0.0.0/8", "192.168.0.0/16"},
			},
		},
		{
			name: "service config with only loadBalancerSourceRanges",
			input: &kgateway.Service{
				LoadBalancerSourceRanges: []string{"172.16.0.0/12"},
			},
			want: &HelmService{
				LoadBalancerSourceRanges: []string{"172.16.0.0/12"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetServiceValues(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
