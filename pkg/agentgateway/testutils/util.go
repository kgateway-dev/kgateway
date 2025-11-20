package testutils

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"istio.io/istio/pilot/test/util"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/util/file"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

func CompareGolden(t test.Failer, content []byte, goldenFile string) {
	util.CompareContent(t, content, rewrite(goldenFile))
}

// rewrite rewrites a subname to having only printable characters and no white
// space.
func rewrite(s string) string {
	b := []byte{}
	for _, r := range s {
		switch {
		case isSpace(r):
			b = append(b, '_')
		case !strconv.IsPrint(r):
			s := strconv.QuoteRune(r)
			b = append(b, s[1:len(s)-1]...)
		default:
			b = append(b, string(r)...)
		}
	}
	return string(b)
}

func isSpace(r rune) bool {
	if r < 0x2000 {
		switch r {
		// Note: not the same as Unicode Z class.
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0, 0x1680:
			return true
		}
	} else {
		if r <= 0x200a {
			return true
		}
		switch r {
		case 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
			return true
		}
	}
	return false
}

func init() {
	// Add our types to Istio since we are using their library
	utilruntime.Must(schemes.AddToScheme(kube.IstioScheme))
}

func GetTestResource[T any](t *testing.T, collection krt.Collection[T]) T {
	t.Helper()
	l := collection.List()
	if len(l) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(l))
	}
	return l[0]
}

var timestampRegex = regexp.MustCompile(`lastTransitionTime:.*`)

// RunForDirectory runs a set of tests against each file in a directory.
// The file should pass in the input YAMLs at the top of the file, and the expected outputs at the bottom of the file split by:
//
// ---
// # Output
// ... the output
//
// The output is generally created by running the test with `REFRESH_GOLDEN=true`.
func RunForDirectory[Status any, Output any](t *testing.T, base string, run func(t *testing.T, ctx plugins.PolicyCtx) (Status, []Output)) {
	for _, f := range file.ReadDirOrFail(t, base) {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			data := file.AsStringOrFail(t, f)
			inputData := data
			idx := strings.Index(data, "---\n# Output")
			if idx != -1 {
				inputData = data[:idx-1]
			}
			ctx := BuildMockPolicyContext(t, []any{inputData})
			st, objs := run(t, ctx)
			o, err := yaml.Marshal(testOutput[Status, Output]{Status: st, Output: objs})
			if err != nil {
				t.Fatalf("failed to marshal output: %v", err)
			}
			o = timestampRegex.ReplaceAll(o, []byte("lastTransitionTime: fake"))
			output := inputData + "\n---\n# Output\n" + string(o)
			if util.Refresh() {
				util.RefreshGoldenFile(t, []byte(output), f)
			} else {
				util.CompareBytes(t, []byte(data), []byte(output), name)
			}
		})
	}
}

type testOutput[Status any, Output any] struct {
	Status Status   `json:"status,omitempty"`
	Output []Output `json:"output"`
}

func RouteInputs(ctx plugins.PolicyCtx) translator.RouteContextInputs {
	opts := krtutil.KrtOptions{}
	return translator.RouteContextInputs{
		Grants: translator.BuildReferenceGrants(translator.ReferenceGrantsCollection(ctx.Collections.ReferenceGrants, opts)),
		// TODO: it would be nice to use the real one
		RouteParents: translator.BuildRouteParents(krt.NewStaticCollection[*translator.GatewayListener](nil, []*translator.GatewayListener{{
			ParentGateway: types.NamespacedName{
				Name:      "test-gateway",
				Namespace: "default",
			},
			ParentObject: translator.ParentKey{
				Kind:      wellknown.GatewayGVK,
				Name:      "test-gateway",
				Namespace: "default",
			},
			ParentInfo: translator.ParentInfo{
				InternalName: "default/test-gateway",
				Protocol:     gwv1.HTTPProtocolType,
				Port:         80,
				SectionName:  "http",
				ParentGateway: types.NamespacedName{
					Name:      "test-gateway",
					Namespace: "default",
				},
				AllowedKinds: []gwv1.RouteGroupKind{
					{
						Group: ptr.Of(gwv1.Group("gateway.networking.k8s.io")),
						Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
					},
					{
						Group: ptr.Of(gwv1.Group("gateway.networking.k8s.io")),
						Kind:  gwv1.Kind(wellknown.GRPCRouteKind),
					},
					{
						Group: ptr.Of(gwv1.Group("gateway.networking.k8s.io")),
						Kind:  gwv1.Kind(wellknown.TLSRouteKind),
					},
					{
						Group: ptr.Of(gwv1.Group("gateway.networking.k8s.io")),
						Kind:  gwv1.Kind(wellknown.TCPRouteKind),
					},
				},
			},
			Valid: true,
		}})),
		Services:        ctx.Collections.Services,
		InferencePools:  ctx.Collections.InferencePools,
		Namespaces:      ctx.Collections.Namespaces,
		Backends:        ctx.Collections.Backends,
		DirectResponses: ctx.Collections.DirectResponses,
		ControllerName:  ctx.Collections.ControllerName,
	}
}

func BuildMockPolicyContext(t test.Failer, inputs []any) plugins.PolicyCtx {
	return plugins.PolicyCtx{
		Krt:         krt.TestingDummyContext{},
		Collections: BuildMockCollection(t, inputs),
	}
}

func BuildMockCollection(t test.Failer, inputs []any) *plugins.AgwCollections {
	mock := krttest.NewMock(t, inputs)
	col := &plugins.AgwCollections{
		Namespaces:           krttest.GetMockCollection[*corev1.Namespace](mock),
		Nodes:                krttest.GetMockCollection[*corev1.Node](mock),
		Pods:                 krttest.GetMockCollection[*corev1.Pod](mock),
		Services:             krttest.GetMockCollection[*corev1.Service](mock),
		Secrets:              krttest.GetMockCollection[*corev1.Secret](mock),
		ConfigMaps:           krttest.GetMockCollection[*corev1.ConfigMap](mock),
		EndpointSlices:       krttest.GetMockCollection[*discovery.EndpointSlice](mock),
		GatewayClasses:       krttest.GetMockCollection[*gwv1.GatewayClass](mock),
		Gateways:             krttest.GetMockCollection[*gwv1.Gateway](mock),
		HTTPRoutes:           krttest.GetMockCollection[*gwv1.HTTPRoute](mock),
		GRPCRoutes:           krttest.GetMockCollection[*gwv1.GRPCRoute](mock),
		TCPRoutes:            krttest.GetMockCollection[*gwv1a2.TCPRoute](mock),
		TLSRoutes:            krttest.GetMockCollection[*gwv1a2.TLSRoute](mock),
		ReferenceGrants:      krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock),
		BackendTLSPolicies:   krttest.GetMockCollection[*gwv1.BackendTLSPolicy](mock),
		XListenerSets:        krttest.GetMockCollection[*gwxv1a1.XListenerSet](mock),
		InferencePools:       krttest.GetMockCollection[*inf.InferencePool](mock),
		Backends:             krttest.GetMockCollection[*v1alpha1.AgentgatewayBackend](mock),
		AgentgatewayPolicies: krttest.GetMockCollection[*v1alpha1.AgentgatewayPolicy](mock),
		DirectResponses:      krttest.GetMockCollection[*v1alpha1.DirectResponse](mock),
		GatewayExtensions:    krttest.GetMockCollection[*v1alpha1.GatewayExtension](mock),
		ControllerName:       wellknown.DefaultAgwControllerName,
		SystemNamespace:      "istio-system",
		ClusterID:            "Kubernetes",
	}
	col.SetupIndexes()
	return col
}
