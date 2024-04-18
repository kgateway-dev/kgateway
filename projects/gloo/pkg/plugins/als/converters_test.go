package als_test

import (
	"strings"
	"testing"

	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

func TestDetectUnusefulCmds(t *testing.T) {

	tests := []struct {
		name                  string
		accesslog             accesslogv3.AccessLog
		hcmReportStr          string
		httpListenerReportStr string
	}{
		{
			name:      "clean accesslog",
			accesslog: mustConvertAccessLogs("basic", &envoyalfile.FileAccessLog{}),
		},

		{
			name: "not at hcm",
			accesslog: mustConvertAccessLogs("basic",
				&envoyalfile.FileAccessLog{
					AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
						LogFormat: &envoycore.SubstitutionFormatString{
							Format: &envoycore.SubstitutionFormatString_TextFormat{
								TextFormat: "%DOWNSTREAM_TRANSPORT_FAILURE_REASON% and some other stuff",
							},
						},
					},
				}),
			hcmReportStr: "DOWNSTREAM_TRANSPORT_FAILURE_REASON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcmErr := als.DetectUnusefulCmds(als.Hcm, []*accesslogv3.AccessLog{&tt.accesslog})
			if hcmErr != nil && !strings.Contains(hcmErr.Error(), tt.hcmReportStr) {
				t.Errorf("expected %v, got %v", tt.hcmReportStr, hcmErr)
			}
			httpListenerErr := als.DetectUnusefulCmds(als.HttpListener, []*accesslogv3.AccessLog{&tt.accesslog})
			if httpListenerErr != nil && !strings.Contains(httpListenerErr.Error(), tt.httpListenerReportStr) {
				t.Errorf("expected %v, got %v", tt.hcmReportStr, hcmErr)
			}
		})
	}

}

func mustConvertAccessLogs(name string, cfg proto.Message) envoyal.AccessLog {
	out, err := translatorutil.NewAccessLogWithConfig(name, cfg)
	if err != nil {
		panic(err)
	}
	return out
}
