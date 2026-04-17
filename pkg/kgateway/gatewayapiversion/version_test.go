package gatewayapiversion

import (
	"strings"
	"testing"
)

func TestParseMinorVersion(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"v1.2.0", "1.2", true},
		{"v1.5.1", "1.5", true},
		{"1.3.0", "1.3", true},
		{"v1.4", "1.4", true},
		{"v1.10.0-rc.1", "1.10", true},
		{"", "", false},
		{"not-a-version", "", false},
		{"v", "", false},
	}
	for _, c := range cases {
		got, ok := parseMinorVersion(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseMinorVersion(%q) = (%q, %v); want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestCheckBundleVersion(t *testing.T) {
	// Supported list comes from the embedded YAML: "1.2", "1.3", "1.4".
	t.Run("supported", func(t *testing.T) {
		for _, v := range []string{"v1.2.0", "v1.3.0", "v1.4.2"} {
			if err := checkBundleVersion(v); err != nil {
				t.Errorf("checkBundleVersion(%q) returned error: %v; want nil", v, err)
			}
		}
	})

	t.Run("unsupported minor returns error with docs link and bypass hint", func(t *testing.T) {
		err := checkBundleVersion("v1.5.1")
		if err == nil {
			t.Fatal("expected error for v1.5.1, got nil")
		}
		if !strings.Contains(err.Error(), DocsURL) {
			t.Errorf("expected error to reference %s; got %q", DocsURL, err.Error())
		}
		if !strings.Contains(err.Error(), "KGW_SKIP_GATEWAY_API_VERSION_CHECK") {
			t.Errorf("expected error to mention bypass env var; got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "v1.5.1") {
			t.Errorf("expected error to name the offending version; got %q", err.Error())
		}
	})

	t.Run("missing annotation", func(t *testing.T) {
		err := checkBundleVersion("")
		if err == nil {
			t.Fatal("expected error for empty bundle version, got nil")
		}
		if !strings.Contains(err.Error(), BundleVersionAnnotation) {
			t.Errorf("expected error to name the annotation; got %q", err.Error())
		}
	})

	t.Run("unparseable annotation", func(t *testing.T) {
		err := checkBundleVersion("garbage")
		if err == nil {
			t.Fatal("expected error for unparseable bundle version, got nil")
		}
		if !strings.Contains(err.Error(), "garbage") {
			t.Errorf("expected error to quote the unparseable value; got %q", err.Error())
		}
	})
}

func TestSupportedVersionsEmbedded(t *testing.T) {
	vs, err := SupportedVersions()
	if err != nil {
		t.Fatalf("SupportedVersions returned error: %v", err)
	}
	// v2.2.x supports 1.2 to 1.4 inclusive — guard against accidental edits.
	want := []string{"1.2", "1.3", "1.4"}
	if len(vs) != len(want) {
		t.Fatalf("supported versions = %v; want %v", vs, want)
	}
	for i, v := range want {
		if vs[i] != v {
			t.Errorf("supported[%d] = %q; want %q", i, vs[i], v)
		}
	}
}
