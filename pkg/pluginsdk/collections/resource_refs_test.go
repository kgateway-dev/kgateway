package collections_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readerCalls are the helpers that read the contents of a Secret or ConfigMap.
// Every call site must be backed by a ResourceRef, otherwise the object is
// never fetched and the read fails as if the user had not created it.
var readerCalls = regexp.MustCompile(
	`\.GetSecret\(|\.GetSecretWithoutRefGrant\(|\.GetSecretsBySelector\(|` +
		`\.GetSecretForRef\(|\.GetConfigMap\(|\.GetConfigMapForRef\(`)

// refDeclaringFiles are the files that build ResourceRef collections. Each
// entry corresponds to one contributor of references.
var refDeclaringFiles = []string{
	"pkg/krtcollections/resourcerefs.go",
	"pkg/krtcollections/gateway_extensions.go",
	"pkg/kgateway/extensions2/pluginutils/headers.go",
	"pkg/kgateway/extensions2/plugins/backend/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/backendconfigpolicy/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/backendtlspolicy/plugin.go",
	"pkg/kgateway/extensions2/plugins/listenerpolicy/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/trafficpolicy/resource_refs.go",
}

// knownReaders is the set of files that read Secret or ConfigMap contents, each
// mapped to the reason it is covered.
//
// This test is a tripwire, not a proof: it cannot tell whether a ref actually
// matches the object a reader asks for. What it does catch is the failure mode
// that a reviewer is most likely to miss -- someone adds a new place that reads
// a Secret and forgets that kgateway no longer keeps every Secret in memory, so
// the read silently fails at runtime instead of at compile time.
//
// If this fails, either add the corresponding ResourceRef and list the file
// here, or explain why no ref is needed.
var knownReaders = map[string]string{
	"pkg/kgateway/translator/listener/gateway_listener_translator.go":     "core listener certs and CA bundles: krtcollections/resourcerefs.go plus the listenerpolicy plugin",
	"pkg/kgateway/gatewaytls/backend.go":                                  "gateway backend client cert: krtcollections/resourcerefs.go",
	"pkg/kgateway/query/query.go":                                         "generic pass-through used by the listener translator and gatewaytls",
	"pkg/kgateway/extensions2/plugins/backend/plugin.go":                  "AWS credentials: backend/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/backendconfigpolicy/tls.go":         "upstream client cert: backendconfigpolicy/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/backendtlspolicy/plugin.go":         "CA bundles: resourceRefs in the same file",
	"pkg/kgateway/extensions2/plugins/trafficpolicy/api_key_auth.go":      "API keys by name and selector: trafficpolicy/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/trafficpolicy/basic_auth_policy.go": "htpasswd secret: trafficpolicy/resource_refs.go",
	"pkg/kgateway/extensions2/plugins/trafficpolicy/oauth2.go":            "client secret and HMAC key: krtcollections/gateway_extensions.go",
	"pkg/kgateway/extensions2/plugins/trafficpolicy/jwt.go":               "local JWKS configmap: krtcollections/gateway_extensions.go",
	"pkg/kgateway/extensions2/pluginutils/headers.go":                     "secret-backed header values: HeaderFilterResourceRefs in the same file",
	"pkg/krtcollections/secrets.go":                                       "the index itself",
	"pkg/krtcollections/configmaps.go":                                    "the index itself",
	"pkg/utils/kubeutils/secrets.go":                                      "helpers that operate on an already-fetched Secret",
}

func TestEveryObjectReaderHasAReferenceSource(t *testing.T) {
	root := repoRoot(t)

	// The declared ref sources must exist; a rename that silently drops one would
	// otherwise go unnoticed.
	for _, f := range refDeclaringFiles {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("declared ResourceRef source %s is missing: %v", f, err)
		}
	}

	var undeclared []string
	for _, dir := range []string{"pkg", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err //nolint:wrapcheck // walk error passthrough
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr //nolint:wrapcheck // walk error passthrough
			}
			if strings.Contains(rel, "/mocks/") {
				return nil
			}
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr //nolint:wrapcheck // walk error passthrough
			}
			if !readerCalls.Match(src) {
				return nil
			}
			if _, ok := knownReaders[rel]; !ok {
				undeclared = append(undeclared, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}

	for _, f := range undeclared {
		t.Errorf("%s reads Secret or ConfigMap contents but is not listed in knownReaders.\n"+
			"kgateway only loads objects that a ResourceRef points at, so this read will fail\n"+
			"at runtime unless something contributes a matching reference. Add the ref (see\n"+
			"pluginsdk.Plugin.ContributesResourceRefs) and then list this file in knownReaders.", f)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
