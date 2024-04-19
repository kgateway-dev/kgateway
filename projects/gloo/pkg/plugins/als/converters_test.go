package als_test

import (
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyalfile "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("Converters", func() {

	DescribeTable("DetectUnusefulCmds",
		func(
			accesslog accesslogv3.AccessLog,
			hcmReportStr string,
			httpListenerReportStr string,
			tcpListenerReportStr string) {

			hcmErr := als.DetectUnusefulCmds(als.Hcm, []*accesslogv3.AccessLog{&accesslog})
			if hcmReportStr == "" {
				Expect(hcmErr).ToNot(HaveOccurred())
			} else {
				Expect(hcmErr.Error()).To(ContainSubstring(hcmReportStr))
			}

			httpListenerErr := als.DetectUnusefulCmds(als.HttpListener, []*accesslogv3.AccessLog{&accesslog})
			if httpListenerReportStr == "" {
				Expect(httpListenerErr).ToNot(HaveOccurred())
			} else {
				Expect(httpListenerErr.Error()).To(ContainSubstring(httpListenerReportStr))
			}

			tcpListenerErr := als.DetectUnusefulCmds(als.Tcp, []*accesslogv3.AccessLog{&accesslog})
			if tcpListenerReportStr == "" {
				Expect(tcpListenerErr).ToNot(HaveOccurred())
			} else {
				Expect(tcpListenerErr.Error()).To(ContainSubstring(tcpListenerReportStr))
			}

		},
		Entry("empty format", mustConvertAccessLogs("basic", &envoyalfile.FileAccessLog{}), "", "", ""),

		Entry("not at hcm", mustConvertAccessLogs("basic",
			&envoyalfile.FileAccessLog{
				AccessLogFormat: &envoyalfile.FileAccessLog_LogFormat{
					LogFormat: &envoycore.SubstitutionFormatString{
						Format: &envoycore.SubstitutionFormatString_TextFormat{
							TextFormat: "%RESP% %DOWNSTREAM_TRANSPORT_FAILURE_REASON% and some other stuff",
						},
					},
				},
			}),
			"DOWNSTREAM_TRANSPORT_FAILURE_REASON", "", "DOWNSTREAM_TRANSPORT_FAILURE_REASON"), // make sure that tcp can report both bad ones

	)

})

func mustConvertAccessLogs(name string, cfg proto.Message) envoyal.AccessLog {
	out, err := translatorutil.NewAccessLogWithConfig(name, cfg)
	if err != nil {
		panic(err)
	}
	return out
}
