package ir

// equality_harness_test.go verifies that the Equals() implementations on core
// IR types detect every exported field mutation. This enforces that adding a
// field without updating Equals fails here instead of silently dropping Envoy
// config updates.
//
// See test/testutils/equalstest for the harness API.

import (
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	equalstest.Run(
		t, baseHarnessPolicyAtt, func(a, b PolicyAtt) bool { return a.Equals(b) }, cases,
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
		PerConnectionBufferLimitBytes: new(uint32(65535)),
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
			// Listeners: length change only; per-field Listener mutations are
			// covered by TestHarnessListenerEquals.
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
				g.PerConnectionBufferLimitBytes = new(uint32(32768))
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
		{
			// Per-field mutations are covered by TestHarnessGatewayValidationBootstrapsEquals.
			Field: "ValidationBootstraps",
			Mutate: func(g *Gateway) {
				g.ValidationBootstraps = &GatewayValidationBootstraps{ReservedPorts: []uint32{8080}}
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

func TestHarnessGatewayValidationBootstrapsEquals(t *testing.T) {
	// Start from a Gateway whose bootstrap sets a subset and listens on its own ports.
	base := func() GatewayValidationBootstraps {
		return GatewayValidationBootstraps{
			Subset:        ValidationBootstrap{Subset: `{"current":true}`},
			ReservedPorts: []uint32{8082, 9091, 19000},
		}
	}
	eq := func(a, b GatewayValidationBootstraps) bool { return a.Equals(&b) }

	// Each field change must be detected.
	equalstest.Run(t, base, eq, []equalstest.Case[GatewayValidationBootstraps]{
		{
			Field:  "Subset",
			Mutate: func(g *GatewayValidationBootstraps) { g.Subset = ValidationBootstrap{Subset: `{"later":true}`} },
		},
		{
			Field:  "ReservedPorts",
			Mutate: func(g *GatewayValidationBootstraps) { g.ReservedPorts = append(g.ReservedPorts, 8443) },
		},
	}, nil)
}

func TestHarnessValidationBootstrapEquals(t *testing.T) {
	// ValidationBootstrap is compared inside GatewayValidationBootstraps.Equals.
	eq := func(a, b ValidationBootstrap) bool {
		return (&GatewayValidationBootstraps{Subset: a}).Equals(&GatewayValidationBootstraps{Subset: b})
	}
	base := func() ValidationBootstrap {
		return ValidationBootstrap{Subset: `{"current":true}`}
	}

	// Bootstrap is parsed from Subset, so only Subset is compared.
	equalstest.Run(t, base, eq, []equalstest.Case[ValidationBootstrap]{
		{
			Field:  "Subset",
			Mutate: func(v *ValidationBootstrap) { v.Subset = `{"later":true}` },
		},
	}, []string{"Bootstrap"})
}

func TestHarnessPodBootstrapEquals(t *testing.T) {
	// Start from a pod running a merged subset on its own ports.
	base := func() PodBootstrap {
		return PodBootstrap{Subset: `{"earlier":true}`, Ports: []uint32{8082, 9091, 19000}}
	}
	eq := func(a, b PodBootstrap) bool { return a.Equals(&b) }

	// Each field change must be detected.
	equalstest.Run(t, base, eq, []equalstest.Case[PodBootstrap]{
		{
			Field:  "Subset",
			Mutate: func(b *PodBootstrap) { b.Subset = `{"later":true}` },
		},
		{
			Field:  "Ports",
			Mutate: func(b *PodBootstrap) { b.Ports = append(b.Ports, 8443) },
		},
	}, nil)

	// A pod without a bootstrap differs from one with it.
	var none *PodBootstrap
	b := base()
	if none.Equals(&b) || b.Equals(none) {
		t.Error("a nil PodBootstrap must not equal a non-nil one")
	}
	if !none.Equals(nil) {
		t.Error("two nil PodBootstraps must be equal")
	}
}

func TestHarnessUniquelyConnectedClientEquals(t *testing.T) {
	// Start from a client of a pod with a merged bootstrap.
	base := func() UniquelyConnectedClient {
		c := NewUniquelyConnectedClient("kgateway-kube-gateway-api~ns~gw", "ns", map[string]string{"pod-template-hash": "a"}, PodLocality{Zone: "z"})
		c.Bootstrap = &PodBootstrap{Subset: `{"earlier":true}`, Ports: []uint32{8082}}
		return c
	}
	eq := func(a, b UniquelyConnectedClient) bool { return a.Equals(b) }

	// Each field change must be detected.
	equalstest.Run(t, base, eq, []equalstest.Case[UniquelyConnectedClient]{
		{Field: "Role", Mutate: func(c *UniquelyConnectedClient) { c.Role = "other" }},
		{Field: "Labels", Mutate: func(c *UniquelyConnectedClient) { c.Labels = map[string]string{"pod-template-hash": "b"} }},
		{Field: "Locality", Mutate: func(c *UniquelyConnectedClient) { c.Locality = PodLocality{Zone: "y"} }},
		{Field: "Namespace", Mutate: func(c *UniquelyConnectedClient) { c.Namespace = "other" }},
		{Field: "KnowsLocalCluster", Mutate: func(c *UniquelyConnectedClient) { c.KnowsLocalCluster = true }},
		{Field: "Bootstrap", Mutate: func(c *UniquelyConnectedClient) {
			c.Bootstrap = &PodBootstrap{Subset: `{"later":true}`, Ports: []uint32{8082}}
		}},
	}, nil)
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
			// Listeners: length change only; per-field Listener mutations are
			// covered by TestHarnessListenerEquals.
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

// baseHarnessFullListener returns a fully-populated Listener for the
// TestHarnessListenerEquals test. It is separate from baseHarnessListener (which
// is minimal and used by Gateway/ListenerSet tests) so that adding fields here
// does not break those tests.
func baseHarnessFullListener() Listener {
	hostname := gwv1.Hostname("example.com")
	port := gwv1.PortNumber(443)
	proto := gwv1.HTTPSProtocolType
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}
	return Listener{
		Listener: gwv1.Listener{
			Name:     "https",
			Hostname: &hostname,
			Port:     port,
			Protocol: proto,
		},
		Parent: &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "my-gateway",
				Namespace:       "default",
				ResourceVersion: "1",
			},
		},
		AttachedPolicies: AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{
				gk: {baseHarnessPolicyAtt()},
			},
		},
		PolicyAncestorRef: gwv1.ParentReference{
			Group:     new(gwv1.Group("gateway.networking.k8s.io")),
			Kind:      new(gwv1.Kind("Gateway")),
			Name:      gwv1.ObjectName("my-gateway"),
			Namespace: new(gwv1.Namespace("default")),
		},
	}
}

func TestHarnessListenerEquals(t *testing.T) {
	cases := []equalstest.Case[Listener]{
		{
			// The embedded gwv1.Listener contributes flattened field names (Name,
			// Hostname, Port, Protocol, TLS, AllowedRoutes, Filters) plus the embedding name
			// "Listener". Cover the embedding via a Port mutation; the flattened
			// names are exempted below.
			Field: "Listener",
			Mutate: func(l *Listener) {
				l.Listener.Port = gwv1.PortNumber(80)
			},
		},
		{
			// Bump the parent's ResourceVersion → versionEquals detects a change
			// (the base parent has Generation 0, so versionEquals falls back to
			// ResourceVersion + UID).
			Field: "Parent",
			Mutate: func(l *Listener) {
				l.Parent = &gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "my-gateway",
						Namespace:       "default",
						ResourceVersion: "2",
					},
				}
			},
		},
		{
			// Add a policy to AttachedPolicies → unequal.
			Field: "AttachedPolicies",
			Mutate: func(l *Listener) {
				gk2 := schema.GroupKind{Group: "other.io", Kind: "OtherPolicy"}
				l.AttachedPolicies.Policies[gk2] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			// Change PolicyAncestorRef.Name → unequal.
			Field: "PolicyAncestorRef",
			Mutate: func(l *Listener) {
				l.PolicyAncestorRef = gwv1.ParentReference{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("Gateway")),
					Name:      gwv1.ObjectName("other-gateway"),
					Namespace: new(gwv1.Namespace("default")),
				}
			},
		},
	}

	// gwv1.Listener is embedded; the harness flattens its exported field names
	// (Name, Hostname, Port, Protocol, TLS, AllowedRoutes, Filters) plus the embedding
	// name "Listener". We cover the embedding via the "Listener" case above;
	// exempt the individual flattened names to avoid requiring redundant cases.
	equalstest.Run(
		t,
		baseHarnessFullListener,
		func(a, b Listener) bool { return a.Equals(b) },
		cases,
		[]string{"Name", "Hostname", "Port", "Protocol", "TLS", "AllowedRoutes", "Filters"}, // embedded gwv1.Listener fields covered by "Listener" case
		// Suppress the gk field from the AttachedPolicies map closure; the
		// "AttachedPolicies" case above exercises it at the struct level.
	)
}

// TestListenerEqualsIgnoresStatusOnlyParentUpdates verifies that a status-only
// write to the parent (which bumps ResourceVersion but not Generation) does NOT
// make two Listeners compare unequal, while a spec change (which bumps
// Generation) does. This mirrors the versionEquals semantics used in
// Listener.Equals to avoid spurious re-translations on status writes.
func TestListenerEqualsIgnoresStatusOnlyParentUpdates(t *testing.T) {
	base := baseHarnessFullListener()
	base.Parent = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-gateway",
			Namespace:       "default",
			UID:             types.UID("my-gateway-uid"),
			ResourceVersion: "1",
			Generation:      1,
		},
	}

	// Status-only write: ResourceVersion bumps, Generation does not → equal.
	statusOnly := baseHarnessFullListener()
	statusOnly.Parent = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-gateway",
			Namespace:       "default",
			UID:             types.UID("my-gateway-uid"),
			ResourceVersion: "2",
			Generation:      1,
		},
	}
	if !base.Equals(statusOnly) {
		t.Error("Listener.Equals returned false for a status-only parent update (ResourceVersion bumped, Generation unchanged); expected true")
	}

	// Spec change: Generation bumps → unequal.
	specChange := baseHarnessFullListener()
	specChange.Parent = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-gateway",
			Namespace:       "default",
			UID:             types.UID("my-gateway-uid"),
			ResourceVersion: "3",
			Generation:      2,
		},
	}
	if base.Equals(specChange) {
		t.Error("Listener.Equals returned true for a parent spec change (Generation bumped); expected false")
	}
}
