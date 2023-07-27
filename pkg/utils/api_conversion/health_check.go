package api_conversion

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/rotisserie/eris"
	envoycluster_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/cluster"
	envoycore_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooOutlierDetectionList(list []*envoy_config_cluster_v3.OutlierDetection) []*envoycluster_gloo.OutlierDetection {
	if list == nil {
		return nil
	}
	result := make([]*envoycluster_gloo.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToGlooOutlierDetection(v)
	}
	return result
}

func ToGlooOutlierDetection(detection *envoy_config_cluster_v3.OutlierDetection) *envoycluster_gloo.OutlierDetection {
	if detection == nil {
		return nil
	}
	return &envoycluster_gloo.OutlierDetection{
		Consecutive_5Xx:                        detection.GetConsecutive_5Xx(),
		Interval:                               detection.GetInterval(),
		BaseEjectionTime:                       detection.GetBaseEjectionTime(),
		MaxEjectionPercent:                     detection.GetMaxEjectionPercent(),
		EnforcingConsecutive_5Xx:               detection.GetEnforcingConsecutive_5Xx(),
		EnforcingSuccessRate:                   detection.GetEnforcingSuccessRate(),
		SuccessRateMinimumHosts:                detection.GetSuccessRateMinimumHosts(),
		SuccessRateRequestVolume:               detection.GetSuccessRateRequestVolume(),
		SuccessRateStdevFactor:                 detection.GetSuccessRateStdevFactor(),
		ConsecutiveGatewayFailure:              detection.GetConsecutiveGatewayFailure(),
		EnforcingConsecutiveGatewayFailure:     detection.GetEnforcingConsecutiveGatewayFailure(),
		SplitExternalLocalOriginErrors:         detection.GetSplitExternalLocalOriginErrors(),
		ConsecutiveLocalOriginFailure:          detection.GetConsecutiveLocalOriginFailure(),
		EnforcingConsecutiveLocalOriginFailure: detection.GetEnforcingConsecutiveLocalOriginFailure(),
		EnforcingLocalOriginSuccessRate:        detection.GetEnforcingLocalOriginSuccessRate(),
	}
}

func ToEnvoyOutlierDetectionList(list []*envoycluster_gloo.OutlierDetection) []*envoy_config_cluster_v3.OutlierDetection {
	if list == nil {
		return nil
	}
	result := make([]*envoy_config_cluster_v3.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToEnvoyOutlierDetection(v)
	}
	return result
}

func ToEnvoyOutlierDetection(detection *envoycluster_gloo.OutlierDetection) *envoy_config_cluster_v3.OutlierDetection {
	if detection == nil {
		return nil
	}
	return &envoy_config_cluster_v3.OutlierDetection{
		Consecutive_5Xx:                        detection.GetConsecutive_5Xx(),
		Interval:                               detection.GetInterval(),
		BaseEjectionTime:                       detection.GetBaseEjectionTime(),
		MaxEjectionPercent:                     detection.GetMaxEjectionPercent(),
		EnforcingConsecutive_5Xx:               detection.GetEnforcingConsecutive_5Xx(),
		EnforcingSuccessRate:                   detection.GetEnforcingSuccessRate(),
		SuccessRateMinimumHosts:                detection.GetSuccessRateMinimumHosts(),
		SuccessRateRequestVolume:               detection.GetSuccessRateRequestVolume(),
		SuccessRateStdevFactor:                 detection.GetSuccessRateStdevFactor(),
		ConsecutiveGatewayFailure:              detection.GetConsecutiveGatewayFailure(),
		EnforcingConsecutiveGatewayFailure:     detection.GetEnforcingConsecutiveGatewayFailure(),
		SplitExternalLocalOriginErrors:         detection.GetSplitExternalLocalOriginErrors(),
		ConsecutiveLocalOriginFailure:          detection.GetConsecutiveLocalOriginFailure(),
		EnforcingConsecutiveLocalOriginFailure: detection.GetEnforcingConsecutiveLocalOriginFailure(),
		EnforcingLocalOriginSuccessRate:        detection.GetEnforcingLocalOriginSuccessRate(),
	}
}

func ToEnvoyHealthCheckList(check []*envoycore_gloo.HealthCheck, secrets *v1.SecretList, secretOptions HeaderSecretOptions) ([]*envoy_config_core_v3.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	result := make([]*envoy_config_core_v3.HealthCheck, len(check))
	for i, v := range check {
		var err error
		result[i], err = ToEnvoyHealthCheck(v, secrets, secretOptions)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func ToEnvoyHealthCheck(check *envoycore_gloo.HealthCheck, secrets *v1.SecretList, secretOptions HeaderSecretOptions) (*envoy_config_core_v3.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	hc := &envoy_config_core_v3.HealthCheck{
		Timeout:                      check.GetTimeout(),
		Interval:                     check.GetInterval(),
		InitialJitter:                check.GetInitialJitter(),
		IntervalJitter:               check.GetIntervalJitter(),
		IntervalJitterPercent:        check.GetIntervalJitterPercent(),
		UnhealthyThreshold:           check.GetUnhealthyThreshold(),
		HealthyThreshold:             check.GetHealthyThreshold(),
		ReuseConnection:              check.GetReuseConnection(),
		NoTrafficInterval:            check.GetNoTrafficInterval(),
		UnhealthyInterval:            check.GetUnhealthyInterval(),
		UnhealthyEdgeInterval:        check.GetUnhealthyEdgeInterval(),
		HealthyEdgeInterval:          check.GetHealthyEdgeInterval(),
		EventLogPath:                 check.GetEventLogPath(),
		AlwaysLogHealthCheckFailures: check.GetAlwaysLogHealthCheckFailures(),
	}
	switch typed := check.GetHealthChecker().(type) {
	case *envoycore_gloo.HealthCheck_TcpHealthCheck_:
		hc.HealthChecker = &envoy_config_core_v3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoy_config_core_v3.HealthCheck_TcpHealthCheck{
				Send:    ToEnvoyPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToEnvoyPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *envoycore_gloo.HealthCheck_HttpHealthCheck_:
		var requestHeadersToAdd, err = ToEnvoyHeaderValueOptionList(typed.HttpHealthCheck.GetRequestHeadersToAdd(), secrets, secretOptions)
		if err != nil {
			return nil, err
		}
		if typed.HttpHealthCheck.GetMethod().Number() == envoy_config_core_v3.RequestMethod_CONNECT.Number() {
			return nil, eris.New("method CONNECT is not allowed on http health checkers")
		}
		httpHealthChecker := &envoy_config_core_v3.HealthCheck_HttpHealthCheck{
			Host:                   typed.HttpHealthCheck.GetHost(),
			Path:                   typed.HttpHealthCheck.GetPath(),
			RequestHeadersToAdd:    requestHeadersToAdd,
			RequestHeadersToRemove: typed.HttpHealthCheck.GetRequestHeadersToRemove(),
			ExpectedStatuses:       ToEnvoyInt64RangeList(typed.HttpHealthCheck.GetExpectedStatuses()),
			Method:                 envoy_config_core_v3.RequestMethod(typed.HttpHealthCheck.GetMethod()),
		}
		if typed.HttpHealthCheck.GetUseHttp2() {
			httpHealthChecker.CodecClientType = envoy_type_v3.CodecClientType_HTTP2
		}
		if typed.HttpHealthCheck.GetServiceName() != "" {
			httpHealthChecker.ServiceNameMatcher = &envoy_type_matcher_v3.StringMatcher{
				MatchPattern: &envoy_type_matcher_v3.StringMatcher_Prefix{
					Prefix: typed.HttpHealthCheck.GetServiceName(),
				},
			}
		}
		hc.HealthChecker = &envoy_config_core_v3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: httpHealthChecker,
		}

	case *envoycore_gloo.HealthCheck_GrpcHealthCheck_:
		var initialMetadata, err = ToEnvoyHeaderValueOptionList(typed.GrpcHealthCheck.GetInitialMetadata(), secrets, secretOptions)
		if err != nil {
			return nil, err
		}
		hc.HealthChecker = &envoy_config_core_v3.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoy_config_core_v3.HealthCheck_GrpcHealthCheck{
				ServiceName:     typed.GrpcHealthCheck.GetServiceName(),
				Authority:       typed.GrpcHealthCheck.GetAuthority(),
				InitialMetadata: initialMetadata,
			},
		}
	case *envoycore_gloo.HealthCheck_CustomHealthCheck_:
		switch typedConfig := typed.CustomHealthCheck.GetConfigType().(type) {
		case *envoycore_gloo.HealthCheck_CustomHealthCheck_TypedConfig:
			hc.HealthChecker = &envoy_config_core_v3.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoy_config_core_v3.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoy_config_core_v3.HealthCheck_CustomHealthCheck_TypedConfig{
						TypedConfig: typedConfig.TypedConfig,
					},
				},
			}
		}
	}
	return hc, nil
}

func ToGlooHealthCheckList(check []*envoy_config_core_v3.HealthCheck) ([]*envoycore_gloo.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	result := make([]*envoycore_gloo.HealthCheck, len(check))
	for i, v := range check {
		var err error
		result[i], err = ToGlooHealthCheck(v)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func ToGlooHealthCheck(check *envoy_config_core_v3.HealthCheck) (*envoycore_gloo.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	hc := &envoycore_gloo.HealthCheck{
		Timeout:                      check.GetTimeout(),
		Interval:                     check.GetInterval(),
		InitialJitter:                check.GetInitialJitter(),
		IntervalJitter:               check.GetIntervalJitter(),
		IntervalJitterPercent:        check.GetIntervalJitterPercent(),
		UnhealthyThreshold:           check.GetUnhealthyThreshold(),
		HealthyThreshold:             check.GetHealthyThreshold(),
		ReuseConnection:              check.GetReuseConnection(),
		NoTrafficInterval:            check.GetNoTrafficInterval(),
		UnhealthyInterval:            check.GetUnhealthyInterval(),
		UnhealthyEdgeInterval:        check.GetUnhealthyEdgeInterval(),
		HealthyEdgeInterval:          check.GetHealthyEdgeInterval(),
		EventLogPath:                 check.GetEventLogPath(),
		AlwaysLogHealthCheckFailures: check.GetAlwaysLogHealthCheckFailures(),
	}
	switch typed := check.GetHealthChecker().(type) {
	case *envoy_config_core_v3.HealthCheck_TcpHealthCheck_:
		hc.HealthChecker = &envoycore_gloo.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoycore_gloo.HealthCheck_TcpHealthCheck{
				Send:    ToGlooPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToGlooPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *envoy_config_core_v3.HealthCheck_HttpHealthCheck_:
		httpHealthChecker := &envoycore_gloo.HealthCheck_HttpHealthCheck{
			Host:                   typed.HttpHealthCheck.GetHost(),
			Path:                   typed.HttpHealthCheck.GetPath(),
			RequestHeadersToAdd:    ToGlooHeaderValueOptionList(typed.HttpHealthCheck.GetRequestHeadersToAdd()),
			RequestHeadersToRemove: typed.HttpHealthCheck.GetRequestHeadersToRemove(),
			ExpectedStatuses:       ToGlooInt64RangeList(typed.HttpHealthCheck.GetExpectedStatuses()),
			Method:                 v3.RequestMethod(typed.HttpHealthCheck.GetMethod()),
		}

		if typed.HttpHealthCheck.GetCodecClientType() == envoy_type_v3.CodecClientType_HTTP2 {
			httpHealthChecker.UseHttp2 = true
		}

		switch typed.HttpHealthCheck.GetServiceNameMatcher().GetMatchPattern().(type) {
		case *envoy_type_matcher_v3.StringMatcher_Prefix:
			httpHealthChecker.ServiceName = typed.HttpHealthCheck.GetServiceNameMatcher().GetPrefix()
		case *envoy_type_matcher_v3.StringMatcher_Exact:
			httpHealthChecker.ServiceName = typed.HttpHealthCheck.GetServiceNameMatcher().GetExact()
		}

		hc.HealthChecker = &envoycore_gloo.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: httpHealthChecker,
		}
	case *envoy_config_core_v3.HealthCheck_GrpcHealthCheck_:
		hc.HealthChecker = &envoycore_gloo.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoycore_gloo.HealthCheck_GrpcHealthCheck{
				ServiceName:     typed.GrpcHealthCheck.GetServiceName(),
				Authority:       typed.GrpcHealthCheck.GetAuthority(),
				InitialMetadata: ToGlooHeaderValueOptionList(typed.GrpcHealthCheck.GetInitialMetadata()),
			},
		}
	case *envoy_config_core_v3.HealthCheck_CustomHealthCheck_:
		switch typedConfig := typed.CustomHealthCheck.GetConfigType().(type) {
		case *envoy_config_core_v3.HealthCheck_CustomHealthCheck_TypedConfig:
			hc.HealthChecker = &envoycore_gloo.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore_gloo.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore_gloo.HealthCheck_CustomHealthCheck_TypedConfig{
						TypedConfig: typedConfig.TypedConfig,
					},
				},
			}
		}
	}
	return hc, nil
}

func ToEnvoyPayloadList(payload []*envoycore_gloo.HealthCheck_Payload) []*envoy_config_core_v3.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	result := make([]*envoy_config_core_v3.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToEnvoyPayload(v)
	}
	return result
}

func ToEnvoyPayload(payload *envoycore_gloo.HealthCheck_Payload) *envoy_config_core_v3.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	var result *envoy_config_core_v3.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *envoycore_gloo.HealthCheck_Payload_Text:
		result = &envoy_config_core_v3.HealthCheck_Payload{
			Payload: &envoy_config_core_v3.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}

func ToGlooPayloadList(payload []*envoy_config_core_v3.HealthCheck_Payload) []*envoycore_gloo.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	result := make([]*envoycore_gloo.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToGlooPayload(v)
	}
	return result
}

func ToGlooPayload(payload *envoy_config_core_v3.HealthCheck_Payload) *envoycore_gloo.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	var result *envoycore_gloo.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *envoy_config_core_v3.HealthCheck_Payload_Text:
		result = &envoycore_gloo.HealthCheck_Payload{
			Payload: &envoycore_gloo.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}
