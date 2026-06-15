package listenerpolicy

import (
	"testing"

	envoyaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestConvertLocalReplyConfig(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		out, err := convertLocalReplyConfig(nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("empty mappers returns nil", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{})
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("status code match maps to status code filter", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500}},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.GetMappers(), 1)

		scf := out.GetMappers()[0].GetFilter().GetStatusCodeFilter()
		require.NotNil(t, scf)
		require.Equal(t, envoyaccesslogv3.ComparisonFilter_GE, scf.GetComparison().GetOp())
		require.Equal(t, uint32(500), scf.GetComparison().GetValue().GetDefaultValue())
	})

	t.Run("status code override", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{
					StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500},
					StatusCode:      new(int32(404)),
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint32(404), out.GetMappers()[0].GetStatusCode().GetValue())
	})

	t.Run("static body set verbatim", func(t *testing.T) {
		body := "<html><body>down</body></html>"
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{
					StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500},
					Body:            new(body),
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, body, out.GetMappers()[0].GetBody().GetInlineString())
		// no content type -> no body format override
		require.Nil(t, out.GetMappers()[0].GetBodyFormatOverride())
	})

	t.Run("content type produces body format override that passes body through", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{
					StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500},
					Body:            new("<html></html>"),
					ContentType:     new("text/html; charset=utf-8"),
				},
			},
		})
		require.NoError(t, err)
		bfo := out.GetMappers()[0].GetBodyFormatOverride()
		require.NotNil(t, bfo)
		require.Equal(t, "text/html; charset=utf-8", bfo.GetContentType())
		// body is rendered verbatim via the %LOCAL_REPLY_BODY% operator
		require.Equal(t, localReplyBodyOperator, bfo.GetTextFormatSource().GetInlineString())
	})

	t.Run("multiple mappers preserve order", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.EQ, Value: 503}, StatusCode: new(int32(404))},
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500}, StatusCode: new(int32(500))},
			},
		})
		require.NoError(t, err)
		require.Len(t, out.GetMappers(), 2)
		require.Equal(t, envoyaccesslogv3.ComparisonFilter_EQ, out.GetMappers()[0].GetFilter().GetStatusCodeFilter().GetComparison().GetOp())
		require.Equal(t, uint32(404), out.GetMappers()[0].GetStatusCode().GetValue())
		require.Equal(t, envoyaccesslogv3.ComparisonFilter_GE, out.GetMappers()[1].GetFilter().GetStatusCodeFilter().GetComparison().GetOp())
	})

	t.Run("invalid op returns error with mapper context", func(t *testing.T) {
		out, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.EQ, Value: 503}},
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.Op("BOGUS"), Value: 500}},
			},
		})
		require.Error(t, err)
		require.Nil(t, out)
		// error identifies which mapper failed
		require.ErrorContains(t, err, "localReplyConfig.mappers[1].statusCodeMatch.op")
	})
}

func TestApplyHCMLocalReplyConfig(t *testing.T) {
	t.Run("nil leaves HCM untouched", func(t *testing.T) {
		pass := &listenerPolicyPluginGwPass{}
		pCtx := &ir.HcmContext{
			Policy: &ListenerPolicyIR{
				defaultPolicy: listenerPolicy{http: &HttpListenerPolicyIr{}},
			},
		}
		out := &envoy_hcm.HttpConnectionManager{}
		require.NoError(t, pass.ApplyHCM(pCtx, out))
		require.Nil(t, out.GetLocalReplyConfig())
	})

	t.Run("config is applied to HCM", func(t *testing.T) {
		lrc, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{
					StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: 500},
					StatusCode:      new(int32(404)),
					Body:            new("oops"),
				},
			},
		})
		require.NoError(t, err)

		pass := &listenerPolicyPluginGwPass{}
		pCtx := &ir.HcmContext{
			Policy: &ListenerPolicyIR{
				defaultPolicy: listenerPolicy{http: &HttpListenerPolicyIr{localReplyConfig: lrc}},
			},
		}
		out := &envoy_hcm.HttpConnectionManager{}
		require.NoError(t, pass.ApplyHCM(pCtx, out))

		require.NotNil(t, out.GetLocalReplyConfig())
		require.Len(t, out.GetLocalReplyConfig().GetMappers(), 1)
		require.Equal(t, uint32(404), out.GetLocalReplyConfig().GetMappers()[0].GetStatusCode().GetValue())
	})
}

func TestHttpListenerPolicyIrEqualsLocalReplyConfig(t *testing.T) {
	mk := func(value int32) *envoy_hcm.LocalReplyConfig {
		lrc, err := convertLocalReplyConfig(&kgateway.LocalReplyConfig{
			Mappers: []kgateway.ResponseMapper{
				{StatusCodeMatch: kgateway.StatusCodeMatcher{Op: kgateway.GE, Value: value}},
			},
		})
		require.NoError(t, err)
		return lrc
	}

	tests := []struct {
		name     string
		ir1      *HttpListenerPolicyIr
		ir2      *HttpListenerPolicyIr
		expected bool
	}{
		{
			name:     "both unset",
			ir1:      &HttpListenerPolicyIr{},
			ir2:      &HttpListenerPolicyIr{},
			expected: true,
		},
		{
			name:     "one set one unset",
			ir1:      &HttpListenerPolicyIr{localReplyConfig: mk(500)},
			ir2:      &HttpListenerPolicyIr{},
			expected: false,
		},
		{
			name:     "equal configs",
			ir1:      &HttpListenerPolicyIr{localReplyConfig: mk(500)},
			ir2:      &HttpListenerPolicyIr{localReplyConfig: mk(500)},
			expected: true,
		},
		{
			name:     "different configs",
			ir1:      &HttpListenerPolicyIr{localReplyConfig: mk(500)},
			ir2:      &HttpListenerPolicyIr{localReplyConfig: mk(400)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.ir1.Equals(tt.ir2))
		})
	}
}
