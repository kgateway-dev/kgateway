package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

func baseHarnessUpstreamTLSValidation() UpstreamTLSValidation {
	return UpstreamTLSValidation{
		CAPEM:              "-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----",
		ServerName:         "example.com",
		InsecureSkipVerify: false,
	}
}

// TestHarnessUpstreamTLSValidationEquals guards change detection on the policy IRs that embed
// this type: rotating the CA has to be observed, or a control-plane client keeps validating
// against the trust material it was first given.
func TestHarnessUpstreamTLSValidationEquals(t *testing.T) {
	cases := []equalstest.Case[UpstreamTLSValidation]{
		{
			Field:  "CAPEM",
			Mutate: func(v *UpstreamTLSValidation) { v.CAPEM = "rotated" },
		},
		{
			Field:  "ServerName",
			Mutate: func(v *UpstreamTLSValidation) { v.ServerName = "other.example.com" },
		},
		{
			Field:  "InsecureSkipVerify",
			Mutate: func(v *UpstreamTLSValidation) { v.InsecureSkipVerify = true },
		},
	}

	equalstest.Run(t, baseHarnessUpstreamTLSValidation,
		func(a, b UpstreamTLSValidation) bool { return a.Equals(&b) }, cases, nil)
}

func TestUpstreamTLSValidationEqualsNilHandling(t *testing.T) {
	var nilValidation *UpstreamTLSValidation

	assert.True(t, nilValidation.Equals(nil), "two absent configs are equal")
	assert.False(t, nilValidation.Equals(&UpstreamTLSValidation{}),
		"an absent config differs from one that explicitly selects the system trust store")
	assert.False(t, (&UpstreamTLSValidation{}).Equals(nil),
		"comparison is symmetric")
	assert.True(t, (&UpstreamTLSValidation{}).Equals(&UpstreamTLSValidation{}),
		"two system-trust-store configs are equal")
}
