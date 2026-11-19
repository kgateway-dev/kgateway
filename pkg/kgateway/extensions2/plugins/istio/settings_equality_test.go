package istio

import "testing"

// TestIstioSettingsEquals pins that the auto-mTLS flag participates in equality
// now that Equals compares fields instead of using ==.
func TestIstioSettingsEquals(t *testing.T) {
	enabled := IstioSettings{EnableAutoMtls: true}
	disabled := IstioSettings{EnableAutoMtls: false}

	if !enabled.Equals(IstioSettings{EnableAutoMtls: true}) {
		t.Error("Equals returned false for two identical IstioSettings")
	}
	if enabled.Equals(disabled) {
		t.Error("Equals returned true for IstioSettings differing by EnableAutoMtls")
	}
	if enabled.Equals("not-istio-settings") {
		t.Error("Equals returned true for a value of a different type")
	}
}
