package common

import (
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
)

func ToEnvoyPercentage(percentage float32) *envoytype.FractionalPercent {
	return &envoytype.FractionalPercent{
		Numerator:   uint32(percentage * 10000),
		Denominator: envoytype.FractionalPercent_MILLION,
	}
}
