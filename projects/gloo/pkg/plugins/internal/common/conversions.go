package common

import (
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
)

func ToEnvoyPercentage(percentage float32) *envoytype.FractionalPercent {
	return &envoytype.FractionalPercent{
		Numerator:   uint32(percentage * 10000),
		Denominator: envoytype.FractionalPercent_MILLION,
	}
}

// use FloatValue to detect when nil (avoids error-prone float comparisons)
func ToEnvoyPercentageWithDefault(percentage *wrappers.FloatValue, defaultValue float32) *envoytype.FractionalPercent {
	if percentage == nil {
		return ToEnvoyPercentage(defaultValue)
	}
	return ToEnvoyPercentage(percentage.GetValue())
}
