package common

import (
	envoytypev2 "github.com/envoyproxy/go-control-plane/envoy/type"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/gogo/protobuf/types"
)

func ToEnvoyPercentage(percentage float32) *envoytype.FractionalPercent {
	return &envoytype.FractionalPercent{
		Numerator:   uint32(percentage * 10000),
		Denominator: envoytype.FractionalPercent_MILLION,
	}
}

func ToEnvoyv2Percentage(percentage float32) *envoytypev2.FractionalPercent {
	return &envoytypev2.FractionalPercent{
		Numerator:   uint32(percentage * 10000),
		Denominator: envoytypev2.FractionalPercent_MILLION,
	}
}

// use FloatValue to detect when nil (avoids error-prone float comparisons)
func ToEnvoyPercentageWithDefault(percentage *types.FloatValue, defaultValue float32) *envoytypev2.FractionalPercent {
	if percentage == nil {
		return ToEnvoyv2Percentage(defaultValue)
	}
	return ToEnvoyv2Percentage(percentage.Value)
}
