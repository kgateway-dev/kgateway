package ir

// equality_harness_test.go verifies that the Equals() implementations on core
// IR types detect every exported field mutation. This enforces that adding a
// field without updating Equals fails here instead of silently dropping Envoy
// config updates.
//
// See test/testutils/equalstest for the harness API.
// Plan 003 must add Listener coverage using this harness once Listener.Equals
// is rewritten (Listener is skipped here per plan 002 scope).

import (
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// harnessTestPolicyIR is a minimal PolicyIR for harness tests.
// (gw_test.go defines mockPolicyIR for its own tests; this is a separate type
// to avoid confusion and allow independent evolution.)
type harnessTestPolicyIR struct {
	val string
}

func (f *harnessTestPolicyIR) CreationTime() time.Time { return time.Time{} }
func (f *harnessTestPolicyIR) Equals(in any) bool {
	other, ok := in.(*harnessTestPolicyIR)
	if !ok {
		return false
	}
	return f.val == other.val
}

func baseHarnessPolicyAtt() PolicyAtt {
	return PolicyAtt{
		GroupKind:  schema.GroupKind{Group: "example.com", Kind: "MyPolicy"},
		Generation: 1,
		PolicyIr:   &harnessTestPolicyIR{val: "base"},
		PolicyRef: &AttachedPolicyRef{
			Group:     "example.com",
			Kind:      "MyPolicy",
			Name:      "my-policy",
			Namespace: "default",
		},
		InheritedPolicyPriority: apiannotations.ShallowMergePreferParent,
		Errors:                  []error{errors.New("base error")},
		HierarchicalPriority:    5,
		PrecedenceWeight:        10,
		// MergeOrigins is +noKrtEquals and listed as exempt below.
	}
}

func TestHarnessPolicyAttEquals(t *testing.T) {
	cases := []equalstest.Case[PolicyAtt]{
		{
			Field:  "GroupKind",
			Mutate: func(p *PolicyAtt) { p.GroupKind = schema.GroupKind{Group: "other.io", Kind: "Other"} },
		},
		{
			Field:  "Generation",
			Mutate: func(p *PolicyAtt) { p.Generation = 99 },
		},
		{
			Field:  "PolicyIr",
			Mutate: func(p *PolicyAtt) { p.PolicyIr = &harnessTestPolicyIR{val: "mutated"} },
		},
		{
			Field:  "PolicyRef",
			Mutate: func(p *PolicyAtt) { p.PolicyRef = &AttachedPolicyRef{Name: "other-policy"} },
		},
		{
			Field:  "InheritedPolicyPriority",
			Mutate: func(p *PolicyAtt) { p.InheritedPolicyPriority = apiannotations.DeepMergePreferChild },
		},
		{
			Field:  "Errors",
			Mutate: func(p *PolicyAtt) { p.Errors = []error{errors.New("different error")} },
		},
		{
			Field:  "HierarchicalPriority",
			Mutate: func(p *PolicyAtt) { p.HierarchicalPriority = 99 },
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(p *PolicyAtt) { p.PrecedenceWeight = 99 },
		},
	}
	equalstest.Run(t, baseHarnessPolicyAtt, func(a, b PolicyAtt) bool { return a.Equals(b) }, cases,
		[]string{"MergeOrigins"}, // +noKrtEquals, gw.go:105
	)
}

func baseHarnessAttachedPolicies() AttachedPolicies {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}
	return AttachedPolicies{
		Policies: map[schema.GroupKind][]PolicyAtt{
			gk: {baseHarnessPolicyAtt()},
		},
	}
}

func TestHarnessAttachedPoliciesEquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}
	gk2 := schema.GroupKind{Group: "other.io", Kind: "OtherPolicy"}

	cases := []equalstest.Case[AttachedPolicies]{
		{
			Field: "Policies",
			// Add a new entry under a different GroupKind.
			Mutate: func(a *AttachedPolicies) {
				a.Policies[gk2] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field: "Policies",
			// Change a PolicyAtt within an existing slice.
			Mutate: func(a *AttachedPolicies) {
				pa := baseHarnessPolicyAtt()
				pa.Generation = 99
				a.Policies[gk] = []PolicyAtt{pa}
			},
		},
		{
			Field: "Policies",
			// Remove an entry.
			Mutate: func(a *AttachedPolicies) {
				delete(a.Policies, gk)
			},
		},
	}

	// AttachedPolicies has a single exported field: Policies (map).
	// All cases target it; no exempt fields needed.
	equalstest.Run(t, baseHarnessAttachedPolicies, func(a, b AttachedPolicies) bool { return a.Equals(b) }, cases, nil)
}

func baseGatewayObj() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-gateway",
			Namespace:       "default",
			ResourceVersion: "1",
			UID:             "uid-1",
		},
	}
}

func baseHarnessListener() Listener {
	port := gwv1.PortNumber(80)
	return Listener{
		Listener: gwv1.Listener{
			Name:     "http",
			Port:     port,
			Protocol: gwv1.HTTPProtocolType,
		},
	}
}

func baseHarnessListenerSet() ListenerSet {
	return ListenerSet{
		ObjectSource: ObjectSource{
			Group:     "gateway.networking.k8s.io",
			Kind:      "ListenerSet",
			Namespace: "default",
			Name:      "my-listenerset",
		},
		Listeners: Listeners{baseHarnessListener()},
		Obj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
				UID:             "ls-uid-1",
			},
		},
		Err: nil,
	}
}

func baseHarnessGateway() Gateway {
	ls := baseHarnessListenerSet()
	gvk := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "ListenerSet"}
	return Gateway{
		ObjectSource: ObjectSource{
			Group:     "gateway.networking.k8s.io",
			Kind:      "Gateway",
			Namespace: "default",
			Name:      "my-gateway",
		},
		Listeners:           Listeners{baseHarnessListener()},
		AllowedListenerSets: GVKListenerSets{gvk: ListenerSets{ls}},
		DeniedListenerSets:  GVKListenerSets{},
		Obj:                 baseGatewayObj(),
		AttachedListenerPolicies: AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{},
		},
		AttachedHttpPolicies: AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{},
		},
		PerConnectionBufferLimitBytes: uint32ptr(65535),
		FrontendTLSConfig:             nil,
		BackendTLSConfig:              nil,
	}
}

func TestHarnessGatewayEquals(t *testing.T) {
	gvk2 := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "OtherSet"}
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[Gateway]{
		{
			// ObjectSource is embedded; cover it by mutating one of its fields.
			Field: "ObjectSource",
			Mutate: func(g *Gateway) {
				g.ObjectSource.Name = "other-gateway"
			},
		},
		{
			// Listeners: length change (field mutations inside a Listener are plan 003's territory).
			Field: "Listeners",
			Mutate: func(g *Gateway) {
				g.Listeners = append(g.Listeners, baseHarnessListener())
			},
		},
		{
			Field: "AllowedListenerSets",
			Mutate: func(g *Gateway) {
				g.AllowedListenerSets[gvk2] = ListenerSets{baseHarnessListenerSet()}
			},
		},
		{
			Field: "DeniedListenerSets",
			Mutate: func(g *Gateway) {
				ls := baseHarnessListenerSet()
				ls.ObjectSource.Name = "denied-set"
				g.DeniedListenerSets[gvk2] = ListenerSets{ls}
			},
		},
		{
			// Obj: mutate ResourceVersion so versionEquals (backend.go:523) detects the change.
			// versionEquals uses ResourceVersion when Generation == 0.
			Field: "Obj",
			Mutate: func(g *Gateway) {
				obj := baseGatewayObj()
				obj.ResourceVersion = "999"
				g.Obj = obj
			},
		},
		{
			Field: "AttachedListenerPolicies",
			Mutate: func(g *Gateway) {
				g.AttachedListenerPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field: "AttachedHttpPolicies",
			Mutate: func(g *Gateway) {
				g.AttachedHttpPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field: "PerConnectionBufferLimitBytes",
			Mutate: func(g *Gateway) {
				g.PerConnectionBufferLimitBytes = uint32ptr(32768)
			},
		},
		{
			Field: "FrontendTLSConfig",
			Mutate: func(g *Gateway) {
				g.FrontendTLSConfig = &FrontendTLSConfigIR{}
			},
		},
		{
			Field: "BackendTLSConfig",
			Mutate: func(g *Gateway) {
				ref := gwv1.SecretObjectReference{Name: "my-secret"}
				g.BackendTLSConfig = &GatewayBackendTLSConfigIR{
					ClientCertificateRef: &ref,
				}
			},
		},
	}

	// ObjectSource is an embedded struct; the harness flattens its exported
	// fields (Group, Kind, Namespace, Name) plus adds the embedding name itself.
	// We cover the embedding via the "ObjectSource" case above; exempt the four
	// individual flattened names to avoid requiring redundant cases.
	equalstest.Run(
		t,
		baseHarnessGateway,
		func(a, b Gateway) bool { return a.Equals(b) },
		cases,
		[]string{"Group", "Kind", "Namespace", "Name"}, // embedded ObjectSource fields covered by "ObjectSource" case
	)
}

func TestHarnessListenerSetEquals(t *testing.T) {
	cases := []equalstest.Case[ListenerSet]{
		{
			// ObjectSource embedding — cover via name mutation.
			Field: "ObjectSource",
			Mutate: func(ls *ListenerSet) {
				ls.ObjectSource.Name = "other-listenerset"
			},
		},
		{
			// Listeners: length change only (per plan 002 scope).
			Field: "Listeners",
			Mutate: func(ls *ListenerSet) {
				ls.Listeners = append(ls.Listeners, baseHarnessListener())
			},
		},
		{
			// Obj: mutate ResourceVersion so versionEquals detects the change.
			// versionEquals uses ResourceVersion when Generation == 0.
			Field: "Obj",
			Mutate: func(ls *ListenerSet) {
				ls.Obj = &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "999",
						UID:             "ls-uid-1",
					},
				}
			},
		},
		{
			Field: "Err",
			Mutate: func(ls *ListenerSet) {
				ls.Err = errors.New("construction failed")
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessListenerSet,
		func(a, b ListenerSet) bool { return a.Equals(b) },
		cases,
		[]string{"Group", "Kind", "Namespace", "Name"}, // embedded ObjectSource fields covered by "ObjectSource" case
	)
}

func baseHarnessBackendRefIR() BackendRefIR {
	backend := NewBackendObjectIR(ObjectSource{
		Namespace: "default",
		Name:      "my-service",
		Kind:      "Service",
	}, 8080, "", "")
	backend.Obj = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-service",
			Namespace:       "default",
			UID:             "svc-uid-1",
			ResourceVersion: "1",
		},
	}
	return BackendRefIR{
		ClusterName:   "service_default_my-service_8080",
		Weight:        1,
		BackendObject: &backend,
		Err:           nil,
	}
}

func TestHarnessBackendRefIREquals(t *testing.T) {
	cases := []equalstest.Case[BackendRefIR]{
		{
			Field:  "ClusterName",
			Mutate: func(b *BackendRefIR) { b.ClusterName = "other_cluster" },
		},
		{
			Field:  "Weight",
			Mutate: func(b *BackendRefIR) { b.Weight = 50 },
		},
		{
			Field:  "BackendObject",
			Mutate: func(b *BackendRefIR) { b.BackendObject = nil },
		},
		{
			Field:  "Err",
			Mutate: func(b *BackendRefIR) { b.Err = errors.New("backend not found") },
		},
	}

	equalstest.Run(t, baseHarnessBackendRefIR, func(a, b BackendRefIR) bool { return a.Equals(b) }, cases, nil)
}

func baseHarnessGatewayExtension() GatewayExtension {
	grpcSvcName := gwv1.ObjectName("ext-auth-svc")
	return GatewayExtension{
		ObjectSource: ObjectSource{
			Group:     "gateway.kgateway.io",
			Kind:      "GatewayExtension",
			Namespace: "default",
			Name:      "my-extension",
		},
		ExtAuth: &kgateway.ExtAuthProvider{
			GrpcService: &kgateway.ExtGrpcService{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: grpcSvcName,
					},
				},
			},
		},
		ExtProc: nil,
		RateLimit: &kgateway.RateLimitProvider{
			Domain: "my-domain",
		},
		JWT:              nil,
		OAuth2:           nil,
		PrecedenceWeight: 5,
	}
}

// TestHarnessGatewayExtensionEquals drives the ObjectSource fix in Step 4.
// The "ObjectSource" mutation case will FAIL until Step 4 adds the comparison.
func TestHarnessGatewayExtensionEquals(t *testing.T) {
	differentSvcName := gwv1.ObjectName("different-svc")
	cases := []equalstest.Case[GatewayExtension]{
		{
			// ObjectSource: mutate Name. This case passes only after Step 4.
			Field: "ObjectSource",
			Mutate: func(e *GatewayExtension) {
				e.ObjectSource.Name = "other-extension"
			},
		},
		{
			Field: "ExtAuth",
			Mutate: func(e *GatewayExtension) {
				e.ExtAuth = &kgateway.ExtAuthProvider{
					GrpcService: &kgateway.ExtGrpcService{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: differentSvcName,
							},
						},
					},
				}
			},
		},
		{
			Field: "ExtProc",
			Mutate: func(e *GatewayExtension) {
				e.ExtProc = &kgateway.ExtProcProvider{}
			},
		},
		{
			Field: "RateLimit",
			Mutate: func(e *GatewayExtension) {
				e.RateLimit = &kgateway.RateLimitProvider{Domain: "other-domain"}
			},
		},
		{
			Field: "JWT",
			Mutate: func(e *GatewayExtension) {
				e.JWT = &kgateway.JWT{}
			},
		},
		{
			Field: "OAuth2",
			Mutate: func(e *GatewayExtension) {
				e.OAuth2 = &kgateway.OAuth2Provider{}
			},
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(e *GatewayExtension) { e.PrecedenceWeight = 99 },
		},
	}

	// ObjectSource is embedded; exempt its flattened field names since we cover
	// them via the "ObjectSource" case.
	equalstest.Run(
		t,
		baseHarnessGatewayExtension,
		func(a, b GatewayExtension) bool { return a.Equals(b) },
		cases,
		[]string{"Group", "Kind", "Namespace", "Name"}, // embedded ObjectSource fields
	)
}

//go:fix inline
func uint32ptr(v uint32) *uint32 { return new(v) }
