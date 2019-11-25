package gogoutils

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/cluster"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/api/v2/core"
)

func ToGlooOutlierDetectionList(list []*envoycluster.OutlierDetection) []*cluster.OutlierDetection {
	result := make([]*cluster.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToGlooOutlierDetection(v)
	}
	return result
}

func ToGlooOutlierDetection(detection *envoycluster.OutlierDetection) *cluster.OutlierDetection {
	return &cluster.OutlierDetection{
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

func ToEnvoyOutlierDetectionList(list []*cluster.OutlierDetection) []*envoycluster.OutlierDetection {
	result := make([]*envoycluster.OutlierDetection, len(list))
	for i, v := range list {
		result[i] = ToEnvoyOutlierDetection(v)
	}
	return result
}

func ToEnvoyOutlierDetection(detection *cluster.OutlierDetection) *envoycluster.OutlierDetection {
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

func ToEnvoyHealthCheckList(check []*core.HealthCheck) []*envoycore.HealthCheck {
	result := make([]*envoycore.HealthCheck, len(check))
	for i, v := range check {
		result[i] = ToEnvoyHealthCheck(v)
	}
	return result
}

func ToEnvoyHealthCheck(check *core.HealthCheck) *envoycore.HealthCheck {
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
	case *core.HealthCheck_TcpHealthCheck_:
		hc.HealthChecker = &envoycore.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &envoycore.HealthCheck_TcpHealthCheck{
				Send:    ToEnvoyPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToEnvoyPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *core.HealthCheck_HttpHealthCheck_:
		hc.HealthChecker = &envoycore.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &envoycore.HealthCheck_HttpHealthCheck{
				Host:                   typed.HttpHealthCheck.GetHost(),
				Path:                   typed.HttpHealthCheck.GetPath(),
				UseHttp2:               typed.HttpHealthCheck.GetUseHttp2(),
				ServiceName:            typed.HttpHealthCheck.GetServiceName(),
				RequestHeadersToAdd:    ToEnvoyHeaderValueOptionList(typed.HttpHealthCheck.GetRequestHeadersToAdd()),
				RequestHeadersToRemove: typed.HttpHealthCheck.GetRequestHeadersToRemove(),
				ExpectedStatuses:       ToEnvoyInt64RangeList(typed.HttpHealthCheck.GetExpectedStatuses()),
			},
		}
	case *core.HealthCheck_GrpcHealthCheck_:
		hc.HealthChecker = &envoycore.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &envoycore.HealthCheck_GrpcHealthCheck{
				ServiceName: typed.GrpcHealthCheck.ServiceName,
				Authority:   typed.GrpcHealthCheck.Authority,
			},
		}
	}
	return hc
}

func ToGlooHealthCheckList(check []*envoycore.HealthCheck) []*core.HealthCheck {
	result := make([]*core.HealthCheck, len(check))
	for i, v := range check {
		result[i] = ToGlooHealthCheck(v)
	}
	return result
}

func ToGlooHealthCheck(check *envoycore.HealthCheck) *core.HealthCheck {
	hc := &core.HealthCheck{
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
		hc.HealthChecker = &core.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &core.HealthCheck_TcpHealthCheck{
				Send:    ToGlooPayload(typed.TcpHealthCheck.GetSend()),
				Receive: ToGlooPayloadList(typed.TcpHealthCheck.GetReceive()),
			},
		}
	case *envoycore.HealthCheck_HttpHealthCheck_:
		hc.HealthChecker = &core.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
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
		hc.HealthChecker = &core.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &core.HealthCheck_GrpcHealthCheck{
				ServiceName: typed.GrpcHealthCheck.ServiceName,
				Authority:   typed.GrpcHealthCheck.Authority,
			},
		}
	}
	return hc
}

func ToEnvoyPayloadList(payload []*core.HealthCheck_Payload) []*envoycore.HealthCheck_Payload {
	result := make([]*envoycore.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToEnvoyPayload(v)
	}
	return result
}

func ToEnvoyPayload(payload *core.HealthCheck_Payload) *envoycore.HealthCheck_Payload {
	var result *envoycore.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *core.HealthCheck_Payload_Text:
		result = &envoycore.HealthCheck_Payload{
			Payload: &envoycore.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}

func ToGlooPayloadList(payload []*envoycore.HealthCheck_Payload) []*core.HealthCheck_Payload {
	result := make([]*core.HealthCheck_Payload, len(payload))
	for i, v := range payload {
		result[i] = ToGlooPayload(v)
	}
	return result
}

func ToGlooPayload(payload *envoycore.HealthCheck_Payload) *core.HealthCheck_Payload {
	var result *core.HealthCheck_Payload
	switch typed := payload.GetPayload().(type) {
	case *envoycore.HealthCheck_Payload_Text:
		result = &core.HealthCheck_Payload{
			Payload: &core.HealthCheck_Payload_Text{
				Text: typed.Text,
			},
		}
	}
	return result
}
