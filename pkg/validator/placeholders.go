package validator

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

// placeholderCert and placeholderKey are a matching self-signed CA pair that
// stand in for TLS files during validation. They are never served.
const placeholderCert = `-----BEGIN CERTIFICATE-----
MIIBhzCCAS2gAwIBAgIBATAKBggqhkjOPQQDAjAqMSgwJgYDVQQDEx9rZ2F0ZXdh
eS12YWxpZGF0aW9uLXBsYWNlaG9sZGVyMCAXDTI2MDEwMTAwMDAwMFoYDzIxMjYw
MTAxMDAwMDAwWjAqMSgwJgYDVQQDEx9rZ2F0ZXdheS12YWxpZGF0aW9uLXBsYWNl
aG9sZGVyMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5gBqfPM3W6MfiwCMR8x5
Okkjg33lp4rcGhvdDA7fO+AsyomPgp6pUU3PWUEAwLG2qFgF7gDx3oet5w79fobf
CqNCMEAwDgYDVR0PAQH/BAQDAgKEMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYE
FAnYfuJnpwrJ4EvakIk1+Pf2Z7EEMAoGCCqGSM49BAMCA0gAMEUCIQC3qOCqLLO2
lXLCYsGh8obg3pA6QCfIvHaTVaijg9SC9wIgNoOA3T1kiReT6hflpU/TOpVoRa7/
pwYzGm5QXqOVQ4E=
-----END CERTIFICATE-----
`

//nolint:gosec // G101: placeholder key used only for local config validation
const placeholderKey = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgDltm9eQn4feb1hgS
ppOieTqxwt1wjgS/cULiGjJG1pGhRANCAATmAGp88zdbox+LAIxHzHk6SSODfeWn
itwaG90MDt874CzKiY+CnqlRTc9ZQQDAsbaoWAXuAPHeh63nDv1+ht8K
-----END PRIVATE KEY-----
`

// replaceTLSFiles swaps TLS certificates, keys, and CAs given as file paths
// for inline placeholders, recursing into Any fields. The files live on the
// proxy, so the validator cannot read them; Envoy reports a bad path or bad
// content when the proxy loads the config. It reports whether m changed.
func replaceTLSFiles(m protoreflect.Message) bool {
	changed := false
	switch v := m.Interface().(type) {
	case *anypb.Any:
		inner, err := v.UnmarshalNew()
		if err != nil || !replaceTLSFiles(inner.ProtoReflect()) {
			return false
		}
		return anypb.MarshalFrom(v, inner, proto.MarshalOptions{Deterministic: true}) == nil
	case *envoytlsv3.TlsCertificate:
		// Replace both so the placeholder pair still matches.
		if isFileDataSource(v.GetCertificateChain()) || isFileDataSource(v.GetPrivateKey()) {
			v.CertificateChain = inlineDataSource(placeholderCert)
			v.PrivateKey = inlineDataSource(placeholderKey)
			changed = true
		}
	case *envoytlsv3.CertificateValidationContext:
		if isFileDataSource(v.GetTrustedCa()) {
			v.TrustedCa = inlineDataSource(placeholderCert)
			changed = true
		}
	}

	m.Range(func(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Message() == nil {
				return true
			}
			val.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
				changed = replaceTLSFiles(mv.Message()) || changed
				return true
			})
		case fd.Message() == nil:
		case fd.IsList():
			list := val.List()
			for i := range list.Len() {
				changed = replaceTLSFiles(list.Get(i).Message()) || changed
			}
		default:
			changed = replaceTLSFiles(val.Message()) || changed
		}
		return true
	})
	return changed
}

func isFileDataSource(ds *envoycorev3.DataSource) bool {
	_, ok := ds.GetSpecifier().(*envoycorev3.DataSource_Filename)
	return ok
}

func inlineDataSource(s string) *envoycorev3.DataSource {
	return &envoycorev3.DataSource{Specifier: &envoycorev3.DataSource_InlineString{InlineString: s}}
}
