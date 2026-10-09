package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/sslutils"
)

func TestGetFrontendTLSConfigCACertificateRefGroup(t *testing.T) {
	const port = gwv1.PortNumber(8443)

	tests := []struct {
		name string
		// group is set on the CA certificate ref of both the default and the per-port validation.
		group gwv1.Group
		// wantValid is true if the ref must be accepted and stored with the core (empty) group.
		wantValid bool
	}{
		{name: "empty group", group: "", wantValid: true},
		{name: "core group", group: "core", wantValid: true},
		{name: "non-core group", group: "example.com", wantValid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := func() *gwv1.FrontendTLSValidation {
				return &gwv1.FrontendTLSValidation{
					CACertificateRefs: []gwv1.ObjectReference{{Group: tt.group, Kind: "Secret", Name: "ca"}},
				}
			}
			in := &gwv1.FrontendTLSConfig{
				Default: gwv1.TLSConfig{Validation: validation()},
				PerPort: []gwv1.TLSPortConfig{{Port: port, TLS: gwv1.TLSConfig{Validation: validation()}}},
			}

			out := getFrontendTLSConfig(in)
			require.NotNil(t, out)

			// Normalization must not mutate the Gateway spec.
			require.Equal(t, tt.group, in.Default.Validation.CACertificateRefs[0].Group)
			require.Equal(t, tt.group, in.PerPort[0].TLS.Validation.CACertificateRefs[0].Group)

			if !tt.wantValid {
				require.ErrorIs(t, out.DefaultError, sslutils.ErrInvalidCACertificateKind)
				require.ErrorIs(t, out.PortErrors[port], sslutils.ErrInvalidCACertificateKind)
				require.Nil(t, out.DefaultValidation)
				require.NotContains(t, out.PerPortValidation, port)
				return
			}

			require.NoError(t, out.DefaultError)
			require.NoError(t, out.PortErrors[port])
			require.NotNil(t, out.DefaultValidation)
			require.Len(t, out.DefaultValidation.CACertificateRefs, 1)
			require.Empty(t, out.DefaultValidation.CACertificateRefs[0].Group)
			require.Contains(t, out.PerPortValidation, port)
			require.Len(t, out.PerPortValidation[port].CACertificateRefs, 1)
			require.Empty(t, out.PerPortValidation[port].CACertificateRefs[0].Group)
		})
	}
}
