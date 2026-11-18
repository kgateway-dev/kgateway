package listener

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestSelectClientCertValidation(t *testing.T) {
	errUnsupportedKind := errors.New("unsupported kind")
	// fetchCA serves the refs named "*-ok" and fails the others with a "<name> not found" error.
	fetchCA := func(r caCertRef) ([]byte, error) {
		name := string(r.ref.Name)
		if strings.HasSuffix(name, "-ok") {
			return []byte(name), nil
		}
		return nil, errors.New(name + " not found")
	}
	policyRef := func(name string) caCertRef {
		return caCertRef{
			ref:           gwv1.ObjectReference{Name: gwv1.ObjectName(name), Kind: "Secret"},
			fromGVK:       wellknown.ListenerPolicyGVK,
			fromNamespace: "policy-ns",
			holderName:    "pol",
		}
	}
	frontendRef := func(name string) caCertRef {
		return caCertRef{
			ref:           gwv1.ObjectReference{Name: gwv1.ObjectName(name), Kind: "Secret"},
			fromGVK:       wellknown.GatewayGVK,
			fromNamespace: "gw-ns",
		}
	}
	policy := func(require bool, refs ...caCertRef) *clientCertSource {
		return &clientCertSource{name: "ListenerPolicy", require: require, refs: refs}
	}
	frontend := func(require bool, refErr error, refs ...caCertRef) *clientCertSource {
		return &clientCertSource{name: "spec.tls.frontend", require: require, allowInsecureFallback: !require, refErr: refErr, refs: refs}
	}

	tests := []struct {
		name    string
		sources []*clientCertSource
		want    *ir.ClientCertificateValidation
		wantOK  bool
		wantErr string
	}{
		{
			name:   "no source",
			wantOK: true,
		},
		{
			name:    "policy replaces the frontend, whose errors are not reported",
			sources: []*clientCertSource{policy(true, policyRef("pol-ok")), frontend(true, errUnsupportedKind, frontendRef("old-ca"))},
			want:    &ir.ClientCertificateValidation{CACertificates: [][]byte{[]byte("pol-ok")}, RequireClientCertificate: true},
			wantOK:  true,
		},
		{
			name:    "required policy without a CA fails the listener with its own error only",
			sources: []*clientCertSource{policy(true, policyRef("pol-missing")), frontend(false, nil, frontendRef("fe-ok"))},
			wantOK:  false,
			wantErr: "ListenerPolicy policy-ns/pol: pol-missing not found",
		},
		{
			name:    "optional policy without a CA falls back to the frontend and reports its error",
			sources: []*clientCertSource{policy(false, policyRef("pol-missing")), frontend(false, nil, frontendRef("fe-ok"))},
			want:    &ir.ClientCertificateValidation{CACertificates: [][]byte{[]byte("fe-ok")}, AllowInsecureFallback: true},
			wantOK:  true,
			wantErr: "ListenerPolicy policy-ns/pol: pol-missing not found",
		},
		{
			name:    "fallback to a frontend without a valid ref fails the listener with the frontend error only",
			sources: []*clientCertSource{policy(false, policyRef("pol-missing")), frontend(true, errUnsupportedKind)},
			wantOK:  false,
			wantErr: "unsupported kind",
		},
		{
			name:    "required frontend without a CA fails the listener",
			sources: []*clientCertSource{nil, frontend(true, nil, frontendRef("old-ca"))},
			wantOK:  false,
			wantErr: "old-ca not found",
		},
		{
			name:    "optional frontend without a CA serves without validation and reports the error",
			sources: []*clientCertSource{nil, frontend(false, nil, frontendRef("old-ca"))},
			wantOK:  true,
			wantErr: "old-ca not found",
		},
		{
			name:    "partially valid source keeps the CAs that could be fetched and reports the others",
			sources: []*clientCertSource{policy(true, policyRef("a-ok"), policyRef("b-missing"), policyRef("c-ok"))},
			want:    &ir.ClientCertificateValidation{CACertificates: [][]byte{[]byte("a-ok"), []byte("c-ok")}, RequireClientCertificate: true},
			wantOK:  true,
			wantErr: "ListenerPolicy policy-ns/pol: b-missing not found",
		},
		{
			name:    "frontend with mixed valid and unsupported refs reports the unsupported ones",
			sources: []*clientCertSource{nil, frontend(true, errUnsupportedKind, frontendRef("fe-ok"))},
			want:    &ir.ClientCertificateValidation{CACertificates: [][]byte{[]byte("fe-ok")}, RequireClientCertificate: true},
			wantOK:  true,
			wantErr: "unsupported kind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := selectClientCertValidation(ir.Listener{}, fetchCA, tt.sources...)
			assert.Equal(t, tt.want, got, "selected validation")
			assert.Equal(t, tt.wantOK, ok, "whether the listener can be served")
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr)
			}
		})
	}
}
