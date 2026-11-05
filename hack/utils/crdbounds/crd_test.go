package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

const crdDir = "../../../install/helm/kgateway-crds/templates"

func crdFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(crdDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no CRDs found in %s", crdDir)
	return files
}

// TestGeneratedCRDsAreBounded fails when hack/generate.sh did not run
// crdbounds, or when a marker sets a bound outside the Go type's range.
func TestGeneratedCRDsAreBounded(t *testing.T) {
	for _, f := range crdFiles(t) {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		v, err := Check(string(b))
		require.NoError(t, err, f)
		assert.Empty(t, v, "%s has unbounded integer schemas; run `make generate-all`", filepath.Base(f))
	}
}

// TestCRDFieldsDecodeSafely walks each API type next to its CRD schema and
// fails on fields the schema alone cannot make decode-safe:
//   - unsigned integers, which controller-gen emits as `format: int32` with no
//     minimum, so a negative number is admitted and then fails to decode;
//   - metav1.Duration, which is a plain string in the schema, so any string is
//     admitted and then fails time.ParseDuration.
func TestCRDFieldsDecodeSafely(t *testing.T) {
	schemas := map[string]*apiextensionsv1.JSONSchemaProps{}
	for _, f := range crdFiles(t) {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		var crd apiextensionsv1.CustomResourceDefinition
		require.NoError(t, yaml.Unmarshal(b, &crd), f)
		for _, v := range crd.Spec.Versions {
			if v.Storage && v.Schema != nil {
				schemas[crd.Spec.Names.Kind] = v.Schema.OpenAPIV3Schema
			}
		}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, kgateway.Install(scheme))
	checked := 0
	for gvk, typ := range scheme.AllKnownTypes() {
		s, ok := schemas[gvk.Kind]
		if !ok {
			continue
		}
		checked++
		w := &walker{}
		w.walk(typ, s, gvk.Kind, 0)
		assert.Empty(t, w.problems, "%s has fields that are admitted but cannot be decoded", gvk.Kind)
	}
	assert.Equal(t, len(schemas), checked, "every CRD should map to a registered Go type")
}

var (
	durationType = reflect.TypeFor[metav1.Duration]()
	opaqueTypes  = map[reflect.Type]bool{
		reflect.TypeFor[metav1.Time]():          true,
		reflect.TypeFor[metav1.ObjectMeta]():    true,
		reflect.TypeFor[resource.Quantity]():    true,
		reflect.TypeFor[intstr.IntOrString]():   true,
		reflect.TypeFor[apiextensionsv1.JSON](): true,
		reflect.TypeFor[runtime.RawExtension](): true,
	}
	unsignedMax = map[reflect.Kind]float64{
		reflect.Uint8: math.MaxUint8, reflect.Uint16: math.MaxUint16,
		reflect.Uint32: math.MaxUint32, reflect.Uint64: math.MaxUint64, reflect.Uint: math.MaxUint64,
	}
)

type walker struct {
	problems []string
}

func (w *walker) fail(path, format string, args ...any) {
	w.problems = append(w.problems, path+": "+fmt.Sprintf(format, args...))
}

func (w *walker) walk(t reflect.Type, s *apiextensionsv1.JSONSchemaProps, path string, depth int) {
	if s == nil || depth > 50 {
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == durationType {
		if s.Pattern == "" && len(s.XValidations) == 0 {
			w.fail(path, "metav1.Duration needs a pattern or CEL rule that only admits Go durations")
		}
		return
	}
	if opaqueTypes[t] {
		return
	}
	if limit, ok := unsignedMax[t.Kind()]; ok {
		if s.Minimum == nil || *s.Minimum < 0 {
			w.fail(path, "%s needs +kubebuilder:validation:Minimum >= 0", t.Kind())
		}
		if s.Maximum == nil || *s.Maximum > limit {
			w.fail(path, "%s needs a +kubebuilder:validation:Maximum within its range", t.Kind())
		}
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		for f := range t.Fields() {
			if !f.IsExported() {
				continue
			}
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if (f.Anonymous && name == "") || strings.Contains(opts, "inline") {
				w.walk(f.Type, s, path, depth+1)
				continue
			}
			if name == "" {
				name = f.Name
			}
			if child, ok := s.Properties[name]; ok {
				w.walk(f.Type, &child, path+"."+name, depth+1)
			}
		}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return
		}
		if s.Items != nil {
			w.walk(t.Elem(), s.Items.Schema, path+"[]", depth+1)
		}
	case reflect.Map:
		if s.AdditionalProperties != nil {
			w.walk(t.Elem(), s.AdditionalProperties.Schema, path+".*", depth+1)
		}
	}
}
