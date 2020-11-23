package gogoutils

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoycluster_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/cluster"
	envoycore_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// Converts between Envoy and Gloo/solokit versions of envoy protos
// This is required because go-control-plane dropped gogoproto in favor of goproto
// in v0.9.0, but solokit depends on gogoproto (and the generated deep equals it creates).
//
// we should work to remove that assumption from solokit and delete this code:
// https://github.com/solo-io/gloo/issues/1793

func ToGlooOutlierDetectionList(list []*envoycluster.OutlierDetection) []*envoycluster_gloo.OutlierDetection {
	if list == nil {
		return nil
	}
	result := make([]*envoycluster_gloo.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToGlooOutlierDetection(v)
	}
	return result
}

func ToGlooOutlierDetection(detection *envoycluster.OutlierDetection) *envoycluster_gloo.OutlierDetection {
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

func ToEnvoyOutlierDetectionList(list []*envoycluster_gloo.OutlierDetection) []*envoycluster.OutlierDetection {
	if list == nil {
		return nil
	}
	result := make([]*envoycluster.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToEnvoyOutlierDetection(v)
	}
	return result
}

func ToEnvoyOutlierDetection(detection *envoycluster_gloo.OutlierDetection) *envoycluster.OutlierDetection {
	if detection == nil {
		return nil
	}
	return &envoycluster.OutlierDetection{
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

func ToEnvoyHealthCheckList(check []*envoycore_gloo.HealthCheck, secrets *v1.SecretList) ([]*envoycore.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	result := make([]*envoycore.HealthCheck, len(check))
	for i, v := range check {
		var err error
		result[i], err = ToEnvoyHealthCheck(v, secrets)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func ToEnvoyHealthCheck(check *envoycore_gloo.HealthCheck, secrets *v1.SecretList) (*envoycore.HealthCheck, error) {
	if check == nil {
		return nil, nil
	}
	hc := &envoycore.HealthCheck{
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
		hc.HealthChecker = &envoycore.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoycore.HealthCheck_TcpHealthCheck{
				Send:    ToEnvoyPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToEnvoyPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *envoycore_gloo.HealthCheck_HttpHealthCheck_:
		var requestHeadersToAdd, err = ToEnvoyHeaderValueOptionList(typed.HttpHealthCheck.GetRequestHeadersToAdd(), secrets)
		if err != nil {
			return nil, err
		}
		hc.HealthChecker = &envoycore.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoycore.HealthCheck_HttpHealthCheck{
				Host:                   typed.HttpHealthCheck.GetHost(),
				Path:                   typed.HttpHealthCheck.GetPath(),
				UseHttp2:               typed.HttpHealthCheck.GetUseHttp2(),
				ServiceName:            typed.HttpHealthCheck.GetServiceName(),
				RequestHeadersToAdd:    requestHeadersToAdd,
				RequestHeadersToRemove: typed.HttpHealthCheck.GetRequestHeadersToRemove(),
				ExpectedStatuses:       ToEnvoyInt64RangeList(typed.HttpHealthCheck.GetExpectedStatuses()),
			},
		}
	case *envoycore_gloo.HealthCheck_GrpcHealthCheck_:
		hc.HealthChecker = &envoycore.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoycore.HealthCheck_GrpcHealthCheck{
				ServiceName: typed.GrpcHealthCheck.ServiceName,
				Authority:   typed.GrpcHealthCheck.Authority,
			},
		}
	case *envoycore_gloo.HealthCheck_CustomHealthCheck_:
		switch typedConfig := typed.CustomHealthCheck.GetConfigType().(type) {
		case *envoycore_gloo.HealthCheck_CustomHealthCheck_Config:
			hc.HealthChecker = &envoycore.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore.HealthCheck_CustomHealthCheck_Config{
						Config: typedConfig.Config,
					},
				},
			}
		case *envoycore_gloo.HealthCheck_CustomHealthCheck_TypedConfig:
			hc.HealthChecker = &envoycore.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore.HealthCheck_CustomHealthCheck_TypedConfig{
						TypedConfig: typedConfig.TypedConfig,
					},
				},
			}
		}
	}
	return hc, nil
}

func ToGlooHealthCheckList(check []*envoycore.HealthCheck) ([]*envoycore_gloo.HealthCheck, error) {
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

func ToGlooHealthCheck(check *envoycore.HealthCheck) (*envoycore_gloo.HealthCheck, error) {
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
	case *envoycore.HealthCheck_TcpHealthCheck_:
		hc.HealthChecker = &envoycore_gloo.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoycore_gloo.HealthCheck_TcpHealthCheck{
				Send:    ToGlooPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToGlooPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *envoycore.HealthCheck_HttpHealthCheck_:
		hc.HealthChecker = &envoycore_gloo.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoycore_gloo.HealthCheck_HttpHealthCheck{
				Host:                   typed.HttpHealthCheck.GetHost(),
				Path:                   typed.HttpHealthCheck.GetPath(),
				UseHttp2:               typed.HttpHealthCheck.GetUseHttp2(),
				ServiceName:            typed.HttpHealthCheck.GetServiceName(),
				RequestHeadersToAdd:    ToGlooHeaderValueOptionList(typed.HttpHealthCheck.GetRequestHeadersToAdd()),
				RequestHeadersToRemove: typed.HttpHealthCheck.GetRequestHeadersToRemove(),
				ExpectedStatuses:       ToGlooInt64RangeList(typed.HttpHealthCheck.GetExpectedStatuses()),
			},
		}
	case *envoycore.HealthCheck_GrpcHealthCheck_:
		hc.HealthChecker = &envoycore_gloo.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoycore_gloo.HealthCheck_GrpcHealthCheck{
				ServiceName: typed.GrpcHealthCheck.ServiceName,
				Authority:   typed.GrpcHealthCheck.Authority,
			},
		}
	case *envoycore.HealthCheck_CustomHealthCheck_:
		switch typedConfig := typed.CustomHealthCheck.GetConfigType().(type) {
		case *envoycore.HealthCheck_CustomHealthCheck_Config:
			hc.HealthChecker = &envoycore_gloo.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore_gloo.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore_gloo.HealthCheck_CustomHealthCheck_Config{
						Config: typedConfig.Config,
					},
				},
			}
		case *envoycore.HealthCheck_CustomHealthCheck_TypedConfig:
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

func ToEnvoyPayloadList(payload []*envoycore_gloo.HealthCheck_Payload) []*envoycore.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	result := make([]*envoycore.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToEnvoyPayload(v)
	}
	return result
}

func ToEnvoyPayload(payload *envoycore_gloo.HealthCheck_Payload) *envoycore.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	var result *envoycore.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *envoycore_gloo.HealthCheck_Payload_Text:
		result = &envoycore.HealthCheck_Payload{
			Payload: &envoycore.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}

func ToGlooPayloadList(payload []*envoycore.HealthCheck_Payload) []*envoycore_gloo.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	result := make([]*envoycore_gloo.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToGlooPayload(v)
	}
	return result
}

func ToGlooPayload(payload *envoycore.HealthCheck_Payload) *envoycore_gloo.HealthCheck_Payload {
	if payload == nil {
		return nil
	}
	var result *envoycore_gloo.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *envoycore.HealthCheck_Payload_Text:
		result = &envoycore_gloo.HealthCheck_Payload{
			Payload: &envoycore_gloo.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}
