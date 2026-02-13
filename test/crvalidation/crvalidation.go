package crvalidation

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"istio.io/istio/pkg/config/crd"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// TestCRValidation tests CEL validation using the passed validator.
// testDataDir is the directory containing the test data.
// Each file in the directory is a yaml file containing documents to be validated.
// The file name is the name of the test, and subtests are named after the object name.
// The test data is split into multiple documents by the '---' delimiter.
// If an error is expected, the error message should be in the _err field of the yaml document, for example:
// ```
// _err: some validation error message
// apiVersion: gateway.kgateway.dev/v1alpha1
// kind: GatewayParameters
// metadata:
//
//	name: example-invalid
//
// spec: {}
// ---
// ... more test cases
// ```

func TestCRValidation(t *testing.T, v *crd.Validator, testDataDir string) {
	d, err := os.ReadDir(testDataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d {
		t.Run(f.Name(), func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(testDataDir, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range splitString(string(content)) {
				obj := &unstructured.Unstructured{}
				if err := yaml.Unmarshal([]byte(item), obj); err != nil {
					t.Fatal(err)
				}
				delete(obj.Object, "_err")
				t.Run(obj.GetName(), func(t *testing.T) {
					want := testExpectation{}
					if err := yaml.Unmarshal([]byte(item), &want); err != nil {
						t.Fatal(err)
					}
					res := v.ValidateCustomResource(obj)
					if want.WantErr == "" {
						// Want no error
						if res != nil {
							t.Fatalf("configuration was invalid: %v", res)
						}
					} else {
						if res == nil {
							t.Fatalf("wanted error like %q, got none", want.WantErr)
						}
						if !strings.Contains(res.Error(), want.WantErr) {
							t.Fatalf("wanted error like %q, got %q", want.WantErr, res)
						}
					}
				})
			}
		})
	}
}

// Split where the '---' appears at the very beginning of a line. This will avoid
// accidentally splitting in cases where yaml resources contain nested yaml (which
// is indented).
var splitRegex = regexp.MustCompile(`(^|\n)---`)

// splitString splits the given yaml doc if it's a multipart document.
func splitString(yamlText string) []string {
	out := make([]string, 0)
	parts := splitRegex.Split(yamlText, -1)
	for _, part := range parts {
		part := strings.TrimSpace(part)
		if len(part) > 0 {
			out = append(out, part)
		}
	}
	return out
}

type testExpectation struct {
	WantErr string `json:"_err,omitempty"`
}
