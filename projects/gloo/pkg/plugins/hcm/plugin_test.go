package hcm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol_upgrade"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
	. "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Plugin", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc

		p            plugins.HttpConnectionManagerPlugin
		pluginParams plugins.Params

		settings *hcm.HttpConnectionManagerSettings
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		p = NewPlugin()
		pluginParams = plugins.Params{
			Ctx: ctx,
		}
		settings = &hcm.HttpConnectionManagerSettings{}
	})

	AfterEach(func() {
		cancel()
	})

	processHcmNetworkFilter := func(cfg *envoyhttp.HttpConnectionManager) error {
		httpListener := &v1.HttpListener{
			Options: &v1.HttpListenerOptions{
				HttpConnectionManagerSettings: settings,
			},
		}
		listener := &v1.Listener{}
		return p.ProcessHcmNetworkFilter(pluginParams, listener, httpListener, cfg)
	}

	It("copy all settings to hcm filter", func() {
		settings = &hcm.HttpConnectionManagerSettings{
			UseRemoteAddress:    &wrappers.BoolValue{Value: false},
			XffNumTrustedHops:   5,
			SkipXffAppend:       true,
			Via:                 "Via",
			GenerateRequestId:   &wrappers.BoolValue{Value: false},
			Proxy_100Continue:   true,
			StreamIdleTimeout:   prototime.DurationToProto(time.Hour),
			IdleTimeout:         prototime.DurationToProto(time.Hour),
			MaxRequestHeadersKb: &wrappers.UInt32Value{Value: 5},
			RequestTimeout:      prototime.DurationToProto(time.Hour),
			DrainTimeout:        prototime.DurationToProto(time.Hour),
			DelayedCloseTimeout: prototime.DurationToProto(time.Hour),
			ServerName:          "ServerName",

			AcceptHttp_10: true,
			HeaderFormat: &hcm.HttpConnectionManagerSettings_ProperCaseHeaderKeyFormat{
				ProperCaseHeaderKeyFormat: true,
			},
			DefaultHostForHttp_10: "DefaultHostForHttp_10",

			// We intentionally do not test tracing as this plugin is not responsible for setting
			// tracing configuration

			ForwardClientCertDetails: hcm.HttpConnectionManagerSettings_APPEND_FORWARD,
			SetCurrentClientCertDetails: &hcm.HttpConnectionManagerSettings_SetCurrentClientCertDetails{
				Subject: &wrappers.BoolValue{Value: true},
				Cert:    true,
				Chain:   true,
				Dns:     true,
				Uri:     true,
			},
			PreserveExternalRequestId: true,

			Upgrades: []*protocol_upgrade.ProtocolUpgradeConfig{
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
			},
			MaxConnectionDuration:        prototime.DurationToProto(time.Hour),
			MaxStreamDuration:            prototime.DurationToProto(time.Hour),
			MaxHeadersCount:              &wrappers.UInt32Value{Value: 5},
			HeadersWithUnderscoresAction: hcm.HttpConnectionManagerSettings_REJECT_CLIENT_REQUEST,
			MaxRequestsPerConnection:     &wrappers.UInt32Value{Value: 5},
			CodecType:                    1,
			ServerHeaderTransformation:   hcm.HttpConnectionManagerSettings_OVERWRITE,
			PathWithEscapedSlashesAction: hcm.HttpConnectionManagerSettings_REJECT_REQUEST,
			AllowChunkedLength:           true,
			EnableTrailers:               true,
			StripAnyHostPort:             true,
			UuidRequestIdConfig: &hcm.HttpConnectionManagerSettings_UuidRequestIdConfigSettings{
				UseRequestIdForTraceSampling: &wrappers.BoolValue{Value: true},
				PackTraceReason:              &wrappers.BoolValue{Value: true},
			},
		}

		cfg := &envoyhttp.HttpConnectionManager{}
		err := processHcmNetworkFilter(cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.UseRemoteAddress).To(Equal(settings.UseRemoteAddress))
		Expect(cfg.XffNumTrustedHops).To(Equal(settings.XffNumTrustedHops))
		Expect(cfg.SkipXffAppend).To(Equal(settings.SkipXffAppend))
		Expect(cfg.Via).To(Equal(settings.Via))
		Expect(cfg.GenerateRequestId).To(Equal(settings.GenerateRequestId))
		Expect(cfg.Proxy_100Continue).To(Equal(settings.Proxy_100Continue))
		Expect(cfg.StreamIdleTimeout).To(MatchProto(settings.StreamIdleTimeout))
		Expect(cfg.MaxRequestHeadersKb).To(MatchProto(settings.MaxRequestHeadersKb))
		Expect(cfg.RequestTimeout).To(MatchProto(settings.RequestTimeout))
		Expect(cfg.DrainTimeout).To(MatchProto(settings.DrainTimeout))
		Expect(cfg.DelayedCloseTimeout).To(MatchProto(settings.DelayedCloseTimeout))
		Expect(cfg.ServerName).To(Equal(settings.ServerName))
		Expect(cfg.HttpProtocolOptions.AcceptHttp_10).To(Equal(settings.AcceptHttp_10))
		Expect(cfg.HttpProtocolOptions.GetHeaderKeyFormat().GetProperCaseWords()).ToNot(BeNil()) // expect proper case words is set
		Expect(cfg.HttpProtocolOptions.GetHeaderKeyFormat().GetStatefulFormatter()).To(BeNil())  // ...which makes stateful formatter nil
		Expect(cfg.HttpProtocolOptions.GetAllowChunkedLength()).To(BeTrue())                     // ...which makes stateful formatter nil
		Expect(cfg.HttpProtocolOptions.GetEnableTrailers()).To(BeTrue())
		Expect(cfg.HttpProtocolOptions.DefaultHostForHttp_10).To(Equal(settings.DefaultHostForHttp_10))
		Expect(cfg.PreserveExternalRequestId).To(Equal(settings.PreserveExternalRequestId))
		Expect(cfg.GetStripAnyHostPort()).To(Equal(settings.StripAnyHostPort))
		Expect(cfg.CommonHttpProtocolOptions).NotTo(BeNil())
		Expect(cfg.CommonHttpProtocolOptions.IdleTimeout).To(MatchProto(settings.IdleTimeout))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxConnectionDuration()).To(MatchProto(settings.MaxConnectionDuration))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxStreamDuration()).To(MatchProto(settings.MaxStreamDuration))
		Expect(cfg.CommonHttpProtocolOptions.GetHeadersWithUnderscoresAction()).To(Equal(envoycore.HttpProtocolOptions_REJECT_REQUEST))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxRequestsPerConnection()).To(MatchProto(settings.MaxRequestsPerConnection))
		Expect(cfg.CommonHttpProtocolOptions.GetMaxHeadersCount()).To(MatchProto(settings.MaxHeadersCount))
		Expect(cfg.GetCodecType()).To(Equal(envoyhttp.HttpConnectionManager_HTTP1))

		Expect(cfg.GetServerHeaderTransformation()).To(Equal(envoyhttp.HttpConnectionManager_OVERWRITE))
		Expect(cfg.GetPathWithEscapedSlashesAction()).To(Equal(envoyhttp.HttpConnectionManager_REJECT_REQUEST))
		Expect(cfg.MergeSlashes).To(Equal(settings.MergeSlashes))
		Expect(cfg.NormalizePath).To(Equal(settings.NormalizePath))

		// Confirm that MockTracingPlugin return the proper value
		Expect(cfg.Tracing).To(BeNil())

		// Expect the UUID request ID config to be set through request_id_extension
		typedConfigOutput := &hcm.HttpConnectionManagerSettings_UuidRequestIdConfigSettings{}
		err = cfg.RequestIdExtension.GetTypedConfig().UnmarshalTo(typedConfigOutput)
		Expect(err).NotTo(HaveOccurred())
		Expect(typedConfigOutput).To(MatchProto(settings.UuidRequestIdConfig))

		Expect(len(cfg.UpgradeConfigs)).To(Equal(1))
		Expect(cfg.UpgradeConfigs[0].UpgradeType).To(Equal("websocket"))
		Expect(cfg.UpgradeConfigs[0].Enabled.GetValue()).To(Equal(true))

		Expect(cfg.ForwardClientCertDetails).To(Equal(envoyhttp.HttpConnectionManager_APPEND_FORWARD))

		ccd := cfg.SetCurrentClientCertDetails
		Expect(ccd.Subject.Value).To(BeTrue())
		Expect(ccd.Cert).To(BeTrue())
		Expect(ccd.Chain).To(BeTrue())
		Expect(ccd.Dns).To(BeTrue())
		Expect(ccd.Uri).To(BeTrue())

	})

	It("should copy stateful_formatter setting to hcm filter", func() {
		settings = &hcm.HttpConnectionManagerSettings{
			HeaderFormat: &hcm.HttpConnectionManagerSettings_PreserveCaseHeaderKeyFormat{
				PreserveCaseHeaderKeyFormat: true,
			},
		}

		cfg := &envoyhttp.HttpConnectionManager{}
		err := processHcmNetworkFilter(cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.HttpProtocolOptions.GetHeaderKeyFormat().GetStatefulFormatter()).ToNot(BeNil()) // expect preserve_case_words to be set
		Expect(cfg.HttpProtocolOptions.GetHeaderKeyFormat().GetProperCaseWords()).To(BeNil())      // ...which makes proper_case_words nil
	})

	It("copy server_header_transformation setting to hcm filter", func() {
		settings = &hcm.HttpConnectionManagerSettings{
			ServerHeaderTransformation: hcm.HttpConnectionManagerSettings_PASS_THROUGH,
		}

		cfg := &envoyhttp.HttpConnectionManager{}
		err := processHcmNetworkFilter(cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.GetServerHeaderTransformation()).To(Equal(envoyhttp.HttpConnectionManager_PASS_THROUGH))
	})

	Context("upgrades", func() {

		var (
			cfg *envoyhttp.HttpConnectionManager
		)

		BeforeEach(func() {
			cfg = &envoyhttp.HttpConnectionManager{}
		})

		It("enables websockets by default", func() {
			err := processHcmNetworkFilter(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("enables websockets by default with no settings", func() {
			err := processHcmNetworkFilter(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(cfg.GetUpgradeConfigs())).To(Equal(1))
			Expect(cfg.GetUpgradeConfigs()[0].UpgradeType).To(Equal("websocket"))
		})

		It("should error when there's a duplicate upgrade config", func() {
			settings.Upgrades = []*protocol_upgrade.ProtocolUpgradeConfig{
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
				{
					UpgradeType: &protocol_upgrade.ProtocolUpgradeConfig_Websocket{
						Websocket: &protocol_upgrade.ProtocolUpgradeConfig_ProtocolUpgradeSpec{
							Enabled: &wrappers.BoolValue{Value: true},
						},
					},
				},
			}

			err := processHcmNetworkFilter(cfg)
			Expect(err).To(MatchError(ContainSubstring("upgrade config websocket is not unique")))
		})

	})

	Context("supported Envoy HCM settings", func() {
		// obtain all field names of a given instance's type
		getTypeFieldsFromInstance := func(instance interface{}) []string {
			instanceValue := reflect.ValueOf(instance)
			instanceType := instanceValue.Type()

			fieldNames := []string{}
			for i := 0; i < instanceValue.NumField(); i++ {
				fieldNames = append(fieldNames, instanceType.Field(i).Name)
			}

			return fieldNames
		}

		It("contains only the fields we expect", func() {
			// read in expected HCM fields from file
			expectedFieldsJsonFile, err := os.Open("testing/expected_hcm_fields.json")
			Expect(err).To(BeNil())
			defer expectedFieldsJsonFile.Close()

			expectedFieldsJsonByteValue, err := ioutil.ReadAll(expectedFieldsJsonFile)
			Expect(err).To(BeNil())

			var expectedFields []string
			json.Unmarshal(expectedFieldsJsonByteValue, &expectedFields)

			expectedFieldsMap := map[string]bool{}
			for _, fieldName := range expectedFields {
				expectedFieldsMap[fieldName] = true
			}

			// Get all of the fields associated with the Envoy HTTP Connection Manager
			hcmFields := getTypeFieldsFromInstance(envoyhttp.HttpConnectionManager{})

			// Record the names of any fields that were not present the last time we updated this test
			newFields := []string{}
			for _, fieldName := range hcmFields {
				_, found := expectedFieldsMap[fieldName]
				if !found {
					newFields = append(newFields, fieldName)
				}
			}

			if len(newFields) > 0 {
				failureMessage := fmt.Sprintf(`
New Fields have been added to the envoy HTTP Connection Manager.
You may want to consider adding support for these fields to Gloo Edge's API.
You can force this test to pass by adding the new fields listed below to projects/gloo/pkg/plugins/hcm/testing/expected_hcm_fields.json
%+v`,
					newFields)
				Fail(failureMessage)
			}
		})
	})
})
