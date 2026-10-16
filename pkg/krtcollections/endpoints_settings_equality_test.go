package krtcollections

import "testing"

// TestEndpointsSettingsEquals pins that the auto-mTLS flag participates in
// equality. EndpointsSettings feeds the endpoint collections, so a settings
// change that did not register would leave endpoint metadata stale.
func TestEndpointsSettingsEquals(t *testing.T) {
	enabled := EndpointsSettings{EnableAutoMtls: true}
	disabled := EndpointsSettings{EnableAutoMtls: false}

	if !enabled.Equals(EndpointsSettings{EnableAutoMtls: true}) {
		t.Error("Equals returned false for two identical EndpointsSettings")
	}
	if enabled.Equals(disabled) {
		t.Error("Equals returned true for EndpointsSettings differing by EnableAutoMtls")
	}
}
