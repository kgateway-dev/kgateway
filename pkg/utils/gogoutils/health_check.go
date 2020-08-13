package gogoutils

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
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
		Consecutive_5Xx:                        UInt32ProtoToGogo(detection.GetConsecutive_5Xx()),
		Interval:                               DurationProtoToGogo(detection.GetInterval()),
		BaseEjectionTime:                       DurationProtoToGogo(detection.GetBaseEjectionTime()),
		MaxEjectionPercent:                     UInt32ProtoToGogo(detection.GetMaxEjectionPercent()),
		EnforcingConsecutive_5Xx:               UInt32ProtoToGogo(detection.GetEnforcingConsecutive_5Xx()),
		EnforcingSuccessRate:                   UInt32ProtoToGogo(detection.GetEnforcingSuccessRate()),
		SuccessRateMinimumHosts:                UInt32ProtoToGogo(detection.GetSuccessRateMinimumHosts()),
		SuccessRateRequestVolume:               UInt32ProtoToGogo(detection.GetSuccessRateRequestVolume()),
		SuccessRateStdevFactor:                 UInt32ProtoToGogo(detection.GetSuccessRateStdevFactor()),
		ConsecutiveGatewayFailure:              UInt32ProtoToGogo(detection.GetConsecutiveGatewayFailure()),
		EnforcingConsecutiveGatewayFailure:     UInt32ProtoToGogo(detection.GetEnforcingConsecutiveGatewayFailure()),
		SplitExternalLocalOriginErrors:         detection.GetSplitExternalLocalOriginErrors(),
		ConsecutiveLocalOriginFailure:          UInt32ProtoToGogo(detection.GetConsecutiveLocalOriginFailure()),
		EnforcingConsecutiveLocalOriginFailure: UInt32ProtoToGogo(detection.GetEnforcingConsecutiveLocalOriginFailure()),
		EnforcingLocalOriginSuccessRate:        UInt32ProtoToGogo(detection.GetEnforcingLocalOriginSuccessRate()),
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
		Consecutive_5Xx:                        UInt32GogoToProto(detection.GetConsecutive_5Xx()),
		Interval:                               DurationGogoToProto(detection.GetInterval()),
		BaseEjectionTime:                       DurationGogoToProto(detection.GetBaseEjectionTime()),
		MaxEjectionPercent:                     UInt32GogoToProto(detection.GetMaxEjectionPercent()),
		EnforcingConsecutive_5Xx:               UInt32GogoToProto(detection.GetEnforcingConsecutive_5Xx()),
		EnforcingSuccessRate:                   UInt32GogoToProto(detection.GetEnforcingSuccessRate()),
		SuccessRateMinimumHosts:                UInt32GogoToProto(detection.GetSuccessRateMinimumHosts()),
		SuccessRateRequestVolume:               UInt32GogoToProto(detection.GetSuccessRateRequestVolume()),
		SuccessRateStdevFactor:                 UInt32GogoToProto(detection.GetSuccessRateStdevFactor()),
		ConsecutiveGatewayFailure:              UInt32GogoToProto(detection.GetConsecutiveGatewayFailure()),
		EnforcingConsecutiveGatewayFailure:     UInt32GogoToProto(detection.GetEnforcingConsecutiveGatewayFailure()),
		SplitExternalLocalOriginErrors:         detection.GetSplitExternalLocalOriginErrors(),
		ConsecutiveLocalOriginFailure:          UInt32GogoToProto(detection.GetConsecutiveLocalOriginFailure()),
		EnforcingConsecutiveLocalOriginFailure: UInt32GogoToProto(detection.GetEnforcingConsecutiveLocalOriginFailure()),
		EnforcingLocalOriginSuccessRate:        UInt32GogoToProto(detection.GetEnforcingLocalOriginSuccessRate()),
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
		Timeout:                      DurationStdToProto(check.GetTimeout()),
		Interval:                     DurationStdToProto(check.GetInterval()),
		InitialJitter:                DurationGogoToProto(check.GetInitialJitter()),
		IntervalJitter:               DurationGogoToProto(check.GetIntervalJitter()),
		IntervalJitterPercent:        check.GetIntervalJitterPercent(),
		UnhealthyThreshold:           UInt32GogoToProto(check.GetUnhealthyThreshold()),
		HealthyThreshold:             UInt32GogoToProto(check.GetHealthyThreshold()),
		ReuseConnection:              BoolGogoToProto(check.GetReuseConnection()),
		NoTrafficInterval:            DurationGogoToProto(check.GetNoTrafficInterval()),
		UnhealthyInterval:            DurationGogoToProto(check.GetUnhealthyInterval()),
		UnhealthyEdgeInterval:        DurationGogoToProto(check.GetUnhealthyEdgeInterval()),
		HealthyEdgeInterval:          DurationGogoToProto(check.GetHealthyEdgeInterval()),
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
			converted, err := protoutils.StructGogoToPb(typedConfig.Config)
			if err != nil {
				return nil, err
			}
			hc.HealthChecker = &envoycore.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore.HealthCheck_CustomHealthCheck_Config{
						Config: converted,
					},
				},
			}
		case *envoycore_gloo.HealthCheck_CustomHealthCheck_TypedConfig:
			converted, err := protoutils.AnyGogoToPb(typedConfig.TypedConfig)
			if err != nil {
				return nil, err
			}
			hc.HealthChecker = &envoycore.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore.HealthCheck_CustomHealthCheck_TypedConfig{
						TypedConfig: converted,
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
		Timeout:                      DurationProtoToStd(check.GetTimeout()),
		Interval:                     DurationProtoToStd(check.GetInterval()),
		InitialJitter:                DurationProtoToGogo(check.GetInitialJitter()),
		IntervalJitter:               DurationProtoToGogo(check.GetIntervalJitter()),
		IntervalJitterPercent:        check.GetIntervalJitterPercent(),
		UnhealthyThreshold:           UInt32ProtoToGogo(check.GetUnhealthyThreshold()),
		HealthyThreshold:             UInt32ProtoToGogo(check.GetHealthyThreshold()),
		ReuseConnection:              BoolProtoToGogo(check.GetReuseConnection()),
		NoTrafficInterval:            DurationProtoToGogo(check.GetNoTrafficInterval()),
		UnhealthyInterval:            DurationProtoToGogo(check.GetUnhealthyInterval()),
		UnhealthyEdgeInterval:        DurationProtoToGogo(check.GetUnhealthyEdgeInterval()),
		HealthyEdgeInterval:          DurationProtoToGogo(check.GetHealthyEdgeInterval()),
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
			converted, err := protoutils.StructPbToGogo(typedConfig.Config)
			if err != nil {
				return nil, err
			}
			hc.HealthChecker = &envoycore_gloo.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore_gloo.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore_gloo.HealthCheck_CustomHealthCheck_Config{
						Config: converted,
					},
				},
			}
		case *envoycore.HealthCheck_CustomHealthCheck_TypedConfig:
			converted, err := protoutils.AnyPbToGogo(typedConfig.TypedConfig)
			if err != nil {
				return nil, err
			}
			hc.HealthChecker = &envoycore_gloo.HealthCheck_CustomHealthCheck_{
				CustomHealthCheck: &envoycore_gloo.HealthCheck_CustomHealthCheck{
					Name: typed.CustomHealthCheck.GetName(),
					ConfigType: &envoycore_gloo.HealthCheck_CustomHealthCheck_TypedConfig{
						TypedConfig: converted,
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
