package server

import (
	"crypto/x509"
	"encoding/pem"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRootGoModEnablesNegativeX509SerialParsing(t *testing.T) {
	const (
		godebugKey   = "x509negativeserial"
		godebugValue = "1"
	)

	settings := defaultGODEBUG(t)
	if !settings.contains(godebugKey, godebugValue) {
		t.Fatalf(
			"root go.mod must contain 'godebug %s=%s' so kgateway can parse legacy certificates with negative serial numbers; test binary DefaultGODEBUG=%q.\nIf go.mod already contains that directive and this test is failing after a Go upgrade, the Go team has disabled the flag; find another workaround.",
			godebugKey,
			godebugValue,
			settings,
		)
	}

	block, _ := pem.Decode([]byte(negativeSerialCertPEM))
	if block == nil {
		t.Fatal("test fixture is not a valid PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf(
			"root go.mod contains 'godebug %s=%s', but Go still rejected a negative-serial certificate: %v\nThe Go team has disabled or removed the %s GODEBUG flag; find another workaround for legacy certificates with negative serial numbers.",
			godebugKey,
			godebugValue,
			err,
			godebugKey,
		)
	}
	if cert.SerialNumber.Sign() >= 0 {
		t.Fatalf("test fixture must have a negative serial number, got %s", cert.SerialNumber)
	}
}

func defaultGODEBUG(t *testing.T) godebugSettings {
	t.Helper()

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("build info is not available; cannot verify root go.mod default GODEBUG settings")
	}
	for _, setting := range buildInfo.Settings {
		if setting.Key == "DefaultGODEBUG" {
			return godebugSettings(setting.Value)
		}
	}
	return ""
}

type godebugSettings string

func (settings godebugSettings) contains(key, value string) bool {
	for _, setting := range strings.Split(string(settings), ",") {
		if setting == key+"="+value {
			return true
		}
	}
	return false
}

const negativeSerialCertPEM = `-----BEGIN CERTIFICATE-----
MIIBBTCBraADAgECAgH/MAoGCCqGSM49BAMCMA0xCzAJBgNVBAMTAjopMB4XDTIy
MDQxNDIzNTYwNFoXDTIyMDQxNTAxNTYwNFowDTELMAkGA1UEAxMCOikwWTATBgcq
hkjOPQIBBggqhkjOPQMBBwNCAAQ9ezsIsj+q17K87z/PXE/rfGRN72P/Wyn5d6oo
5M0ZbSatuntMvfKdX79CQxXAxN4oXk3Aov4jVSG12AcDI8ShMAoGCCqGSM49BAMC
A0cAMEQCIBzfBU5eMPT6m5lsR6cXaJILpAaiD9YxOl4v6dT3rzEjAiBHmjnHmAss
RqUAyJKFzqZxOlK2q4j2IYnuj5+LrLGbQA==
-----END CERTIFICATE-----`
