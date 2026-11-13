package listenerpolicy

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
)

// Envoy configures local-reply headers with its route HeaderParser, so a Host
// or malformed substitution there NACKs the whole listener. Reject the policy
// instead.
func TestLocalReplyMapperHeadersMatchEnvoyHeaderParser(t *testing.T) {
	header := func(name, value string) shared.HTTPHeader {
		n := gwv1.HTTPHeaderName(name)
		return shared.HTTPHeader{Name: &n, Value: &value}
	}
	for _, tc := range []struct {
		name    string
		filter  shared.HTTPHeaderFilter
		wantErr string
	}{
		{name: "valid", filter: shared.HTTPHeaderFilter{
			Add: []shared.HTTPHeader{header("x-code", "%RESPONSE_CODE%")},
			Set: []shared.HTTPHeader{header("x-literal", "100%%")},
		}},
		{name: "set host", filter: shared.HTTPHeaderFilter{Set: []shared.HTTPHeader{header("Host", "a.example.com")}}, wantErr: `cannot set header "Host"`},
		{name: "add host", filter: shared.HTTPHeaderFilter{Add: []shared.HTTPHeader{header("host", "a.example.com")}}, wantErr: `cannot set header "host"`},
		{name: "malformed substitution", filter: shared.HTTPHeaderFilter{Add: []shared.HTTPHeader{header("x-id", "%REQ(x-id")}}, wantErr: `invalid value for header "x-id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapper := kgateway.LocalReplyMapper{Headers: &tc.filter}
			out, err := translateLocalReplyBodyMapper(nil, krtcollections.From{Namespace: "default"}, nil, mapper)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Len(t, out.GetHeadersToAdd(), 2)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, out)
		})
	}
}
