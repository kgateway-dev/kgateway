package faultinjection

import (
	"reflect"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"

	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"

	"testing"
)

func TestToEnvoyPercentage(t *testing.T) {
	assertEqualPercent(.0001, 1, t) // from the docs
	assertEqualPercent(1, 10000, t)
	assertEqualPercent(50.0005, 500005, t)
	assertEqualPercent(100, 1000000, t)
	// assertEqualPercent(50.000005, 50000005, t) cannot test for this level of precision
}

func assertEqualPercent(actual float32, expectedNumerator uint32, t *testing.T) {
	expectedPercentage := envoytype.FractionalPercent{
		Numerator:   expectedNumerator,
		Denominator: envoytype.FractionalPercent_MILLION,
	}

	actualPercentage := common.ToEnvoyPercentage(actual)
	if !reflect.DeepEqual(expectedPercentage, *actualPercentage) {
		t.Errorf("Expected %v but got %v.", expectedPercentage, actualPercentage)
	}
}
