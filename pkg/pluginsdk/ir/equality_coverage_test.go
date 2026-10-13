package ir

// equality_coverage_test.go runs the equalstest harness over the IR types whose
// Equals implementations used to have untracked field gaps.
//
// See test/testutils/equalstest for the harness API.

import (
	"errors"
	"testing"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

// errTestRuleResolution stands in for an error recorded during IR construction.
var errTestRuleResolution = errors.New("backend resolution failed")

// MARK: routes

// embeddedObjectSourceFields are the ObjectSource fields the harness flattens;
// each test covers the embedding as a whole via an "ObjectSource" case.
var embeddedObjectSourceFields = []string{"Group", "Kind", "Namespace", "Name"}

func baseHarnessHTTPRouteObj() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-route",
			Namespace:  "default",
			UID:        "route-uid-1",
			Generation: 1,
		},
		Spec: gwv1.HTTPRouteSpec{
			Hostnames: []gwv1.Hostname{"example.com"},
		},
	}
}

func baseHarnessHttpRouteIR() HttpRouteIR {
	return HttpRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1.GroupVersion.Group,
			Kind:      "HTTPRoute",
			Namespace: "default",
			Name:      "my-route",
		},
		SourceObject: baseHarnessHTTPRouteObj(),
		ParentRefs:   []gwv1.ParentReference{{Name: "my-gateway"}},
		Hostnames:    []string{"example.com"},
		AttachedPolicies: AttachedPolicies{
			Policies: map[schema.GroupKind][]PolicyAtt{},
		},
		Rules: []HttpRouteRuleIR{{
			Name:             "rule-0",
			ExtensionRefs:    AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
			AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
			Backends: []HttpBackendOrDelegate{{
				AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
				Backend:          new(baseHarnessBackendRefIR()),
			}},
		}},
		PrecedenceWeight:               0,
		DelegationInheritParentMatcher: false,
	}
}

func TestHarnessHttpRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[HttpRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *HttpRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			// SourceObject: a spec edit bumps generation, which versionEquals compares.
			Field: "SourceObject",
			Mutate: func(r *HttpRouteIR) {
				obj := baseHarnessHTTPRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field:  "ParentRefs",
			Mutate: func(r *HttpRouteIR) { r.ParentRefs[0].Name = "other-gateway" },
		},
		{
			Field:  "Hostnames",
			Mutate: func(r *HttpRouteIR) { r.Hostnames = []string{"other.example.com"} },
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *HttpRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			// Rules: resolved backends live here and are not covered by the source
			// object's generation, so rulesEqual must compare them.
			Field: "Rules",
			Mutate: func(r *HttpRouteIR) {
				r.Rules[0].Backends[0].Backend.ClusterName = "blackhole_cluster"
			},
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(r *HttpRouteIR) { r.PrecedenceWeight = 10 },
		},
		{
			Field:  "DelegationInheritParentMatcher",
			Mutate: func(r *HttpRouteIR) { r.DelegationInheritParentMatcher = true },
		},
	}

	equalstest.Run(
		t,
		baseHarnessHttpRouteIR,
		func(a, b HttpRouteIR) bool { return a.Equals(b) },
		cases,
		embeddedObjectSourceFields,
	)
}

// TestHarnessHttpRouteIRRulesEquals covers the rest of what rulesEqual is
// responsible for: everything in a rule that is resolved from objects other
// than the route itself, and so cannot ride on the route's generation.
func TestHarnessHttpRouteIRRulesEquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	for _, tc := range []struct {
		name   string
		mutate func(*HttpRouteIR)
	}{
		{
			name:   "rule count",
			mutate: func(r *HttpRouteIR) { r.Rules = append(r.Rules, HttpRouteRuleIR{}) },
		},
		{
			name: "rule attached policies",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name: "rule extension refs",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].ExtensionRefs.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name:   "backend count",
			mutate: func(r *HttpRouteIR) { r.Rules[0].Backends = nil },
		},
		{
			name: "backend policies",
			mutate: func(r *HttpRouteIR) {
				r.Rules[0].Backends[0].AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			name:   "rule error",
			mutate: func(r *HttpRouteIR) { r.Rules[0].Err = errTestRuleResolution },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := baseHarnessHttpRouteIR()
			mutated := baseHarnessHttpRouteIR()
			tc.mutate(&mutated)

			if orig.Equals(mutated) {
				t.Errorf("Equals returned true after changing %s; rulesEqual must detect it", tc.name)
			}
		})
	}
}

func baseHarnessTCPRouteObj() *gwv1a2.TCPRoute {
	return &gwv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-tcp-route",
			Namespace:  "default",
			UID:        "tcp-route-uid-1",
			Generation: 1,
		},
	}
}

func baseHarnessTcpRouteIR() TcpRouteIR {
	return TcpRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1a2.GroupVersion.Group,
			Kind:      "TCPRoute",
			Namespace: "default",
			Name:      "my-tcp-route",
		},
		SourceObject:     baseHarnessTCPRouteObj(),
		ParentRefs:       []gwv1.ParentReference{{Name: "my-gateway"}},
		AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
		Backends:         []BackendRefIR{baseHarnessBackendRefIR()},
	}
}

func TestHarnessTcpRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[TcpRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *TcpRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			Field: "SourceObject",
			Mutate: func(r *TcpRouteIR) {
				obj := baseHarnessTCPRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field:  "ParentRefs",
			Mutate: func(r *TcpRouteIR) { r.ParentRefs[0].Name = "other-gateway" },
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *TcpRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field:  "Backends",
			Mutate: func(r *TcpRouteIR) { r.Backends[0].ClusterName = "blackhole_cluster" },
		},
	}

	equalstest.Run(
		t,
		baseHarnessTcpRouteIR,
		func(a, b TcpRouteIR) bool { return a.Equals(b) },
		cases,
		embeddedObjectSourceFields,
	)
}

func baseHarnessTLSRouteObj() *gwv1a2.TLSRoute {
	return &gwv1a2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-tls-route",
			Namespace:  "default",
			UID:        "tls-route-uid-1",
			Generation: 1,
		},
		Spec: gwv1a2.TLSRouteSpec{
			Hostnames: []gwv1.Hostname{"example.com"},
		},
	}
}

func baseHarnessTlsRouteIR() TlsRouteIR {
	return TlsRouteIR{
		ObjectSource: ObjectSource{
			Group:     gwv1a2.GroupVersion.Group,
			Kind:      "TLSRoute",
			Namespace: "default",
			Name:      "my-tls-route",
		},
		SourceObject:     baseHarnessTLSRouteObj(),
		ParentRefs:       []gwv1.ParentReference{{Name: "my-gateway"}},
		Hostnames:        []string{"example.com"},
		AttachedPolicies: AttachedPolicies{Policies: map[schema.GroupKind][]PolicyAtt{}},
		Backends:         []BackendRefIR{baseHarnessBackendRefIR()},
	}
}

func TestHarnessTlsRouteIREquals(t *testing.T) {
	gk := schema.GroupKind{Group: "example.com", Kind: "MyPolicy"}

	cases := []equalstest.Case[TlsRouteIR]{
		{
			Field:  "ObjectSource",
			Mutate: func(r *TlsRouteIR) { r.ObjectSource.Name = "other-route" },
		},
		{
			Field: "SourceObject",
			Mutate: func(r *TlsRouteIR) {
				obj := baseHarnessTLSRouteObj()
				obj.Generation = 2
				r.SourceObject = obj
			},
		},
		{
			Field:  "ParentRefs",
			Mutate: func(r *TlsRouteIR) { r.ParentRefs[0].Name = "other-gateway" },
		},
		{
			Field:  "Hostnames",
			Mutate: func(r *TlsRouteIR) { r.Hostnames = []string{"other.example.com"} },
		},
		{
			Field: "AttachedPolicies",
			Mutate: func(r *TlsRouteIR) {
				r.AttachedPolicies.Policies[gk] = []PolicyAtt{baseHarnessPolicyAtt()}
			},
		},
		{
			Field:  "Backends",
			Mutate: func(r *TlsRouteIR) { r.Backends[0].ClusterName = "blackhole_cluster" },
		},
	}

	equalstest.Run(
		t,
		baseHarnessTlsRouteIR,
		func(a, b TlsRouteIR) bool { return a.Equals(b) },
		cases,
		embeddedObjectSourceFields,
	)
}

// MARK: policy wrapper

// baseHarnessPolicyObj models the examples/plugin case: a ConfigMap-backed
// policy. ConfigMaps have no generation, so versionEquals compares resourceVersion.
func baseHarnessPolicyObj() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-policy",
			Namespace:       "default",
			UID:             "policy-uid-1",
			ResourceVersion: "1",
		},
	}
}

func baseHarnessPolicyWrapper() PolicyWrapper {
	return PolicyWrapper{
		ObjectSource: ObjectSource{
			Group:     "gateway.kgateway.dev",
			Kind:      "TrafficPolicy",
			Namespace: "default",
			Name:      "my-policy",
		},
		Policy:   baseHarnessPolicyObj(),
		Errors:   nil,
		PolicyIR: &harnessTestPolicyIR{val: "base"},
		TargetRefs: []PolicyRef{{
			Group:       gwv1.GroupName,
			Kind:        "HTTPRoute",
			Name:        "my-route",
			MatchLabels: map[string]string{"app": "my-app"},
		}},
		PrecedenceWeight: 0,
	}
}

func TestHarnessPolicyWrapperEquals(t *testing.T) {
	cases := []equalstest.Case[PolicyWrapper]{
		{
			Field:  "ObjectSource",
			Mutate: func(p *PolicyWrapper) { p.ObjectSource.Name = "other-policy" },
		},
		{
			Field: "Policy",
			Mutate: func(p *PolicyWrapper) {
				obj := baseHarnessPolicyObj()
				obj.ResourceVersion = "2"
				p.Policy = obj
			},
		},
		{
			Field:  "Errors",
			Mutate: func(p *PolicyWrapper) { p.Errors = []error{errTestRuleResolution} },
		},
		{
			Field:  "PolicyIR",
			Mutate: func(p *PolicyWrapper) { p.PolicyIR = &harnessTestPolicyIR{val: "changed"} },
		},
		{
			// The policy object's version is unchanged: targets resolved from
			// somewhere other than the policy object must still be observed.
			Field:  "TargetRefs",
			Mutate: func(p *PolicyWrapper) { p.TargetRefs[0].Name = "other-route" },
		},
		{
			Field:  "PrecedenceWeight",
			Mutate: func(p *PolicyWrapper) { p.PrecedenceWeight = 7 },
		},
	}

	equalstest.Run(
		t,
		baseHarnessPolicyWrapper,
		func(a, b PolicyWrapper) bool { return a.Equals(b) },
		cases,
		embeddedObjectSourceFields,
	)
}

func TestPolicyWrapperEqualsComparesTargetRefFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*PolicyRef)
	}{
		{name: "group", mutate: func(r *PolicyRef) { r.Group = "other.example.com" }},
		{name: "kind", mutate: func(r *PolicyRef) { r.Kind = "Gateway" }},
		{name: "section name", mutate: func(r *PolicyRef) { r.SectionName = "http" }},
		{name: "match labels", mutate: func(r *PolicyRef) { r.MatchLabels["app"] = "other-app" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := baseHarnessPolicyWrapper()
			mutated := baseHarnessPolicyWrapper()
			tc.mutate(&mutated.TargetRefs[0])

			if orig.Equals(mutated) {
				t.Errorf("Equals returned true after changing the target ref %s", tc.name)
			}
		})
	}
}

// MARK: secret

func baseHarnessSecretObj() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-secret",
			Namespace:       "default",
			UID:             "secret-uid-1",
			ResourceVersion: "1",
		},
		Data: map[string][]byte{"tls.key": []byte("original")},
	}
}

func baseHarnessSecret() Secret {
	obj := baseHarnessSecretObj()
	return Secret{
		ObjectSource: ObjectSource{
			Kind:      "Secret",
			Namespace: "default",
			Name:      "my-secret",
		},
		Obj:  obj,
		Data: obj.Data,
	}
}

func TestHarnessSecretEquals(t *testing.T) {
	cases := []equalstest.Case[Secret]{
		{
			Field:  "ObjectSource",
			Mutate: func(s *Secret) { s.ObjectSource.Name = "other-secret" },
		},
		{
			Field: "Obj",
			Mutate: func(s *Secret) {
				obj := baseHarnessSecretObj()
				obj.ResourceVersion = "2"
				s.Obj = obj
			},
		},
		{
			Field:  "Data",
			Mutate: func(s *Secret) { s.Data = map[string][]byte{"tls.key": []byte("rotated")} },
		},
	}

	equalstest.Run(
		t,
		baseHarnessSecret,
		func(a, b Secret) bool { return a.Equals(b) },
		cases,
		embeddedObjectSourceFields,
	)
}

// TestSecretEqualsObservesRotationUnderFixedGeneration covers a Secret created
// with metadata.generation set: the API server keeps that generation across
// updates, so versionEquals never consults resourceVersion and a key rotation
// is visible only through Data.
func TestSecretEqualsObservesRotationUnderFixedGeneration(t *testing.T) {
	objWithGeneration := func(resourceVersion string, key string) *corev1.Secret {
		obj := baseHarnessSecretObj()
		obj.Generation = 1
		obj.ResourceVersion = resourceVersion
		obj.Data = map[string][]byte{"tls.key": []byte(key)}
		return obj
	}
	src := baseHarnessSecret().ObjectSource
	origObj := objWithGeneration("1", "original")
	rotatedObj := objWithGeneration("2", "rotated")
	orig := Secret{ObjectSource: src, Obj: origObj, Data: origObj.Data}
	rotated := Secret{ObjectSource: src, Obj: rotatedObj, Data: rotatedObj.Data}

	if orig.Equals(rotated) {
		t.Error("Equals returned true for a rotated secret whose generation did not change")
	}
}

// MARK: endpoints

func harnessEndpoint(address string) EndpointWithMd {
	return EndpointWithMd{
		LbEndpoint: &envoyendpointv3.LbEndpoint{
			HostIdentifier: &envoyendpointv3.LbEndpoint_Endpoint{
				Endpoint: &envoyendpointv3.Endpoint{
					Hostname: address,
				},
			},
		},
		EndpointMd: EndpointMetadata{Labels: map[string]string{"pod": address}},
	}
}

func baseHarnessEndpointsForBackend() EndpointsForBackend {
	eps := NewEndpointsForBackend(backendWithLabelsForHashTest(map[string]string{
		"app":     "my-app",
		"version": "v1",
		"tier":    "backend",
	}))
	eps.Add(PodLocality{Region: "us-east-1", Zone: "us-east-1a"}, harnessEndpoint("10.0.0.1"))
	return *eps
}

func TestHarnessEndpointsForBackendEquals(t *testing.T) {
	cases := []equalstest.Case[EndpointsForBackend]{
		{
			Field:  "ClusterName",
			Mutate: func(e *EndpointsForBackend) { e.ClusterName = "other_cluster" },
		},
		{
			Field:  "UpstreamResourceName",
			Mutate: func(e *EndpointsForBackend) { e.UpstreamResourceName = "other-upstream" },
		},
		{
			Field:  "Port",
			Mutate: func(e *EndpointsForBackend) { e.Port = 9090 },
		},
		{
			Field:  "Hostname",
			Mutate: func(e *EndpointsForBackend) { e.Hostname = "other.svc.cluster.local" },
		},
		{
			Field:  "TrafficDistribution",
			Mutate: func(e *EndpointsForBackend) { e.TrafficDistribution = wellknown.TrafficDistributionPreferSameZone },
		},
		{
			Field:  "LbEpsEqualityHash",
			Mutate: func(e *EndpointsForBackend) { e.LbEpsEqualityHash++ },
		},
		{
			// LbEps is +noKrtEquals: Add() maintains epsEqualityHash, which is compared.
			Field: "LbEps",
			Mutate: func(e *EndpointsForBackend) {
				e.Add(PodLocality{Region: "us-east-1", Zone: "us-east-1b"}, harnessEndpoint("10.0.0.2"))
			},
		},
	}

	equalstest.Run(
		t,
		baseHarnessEndpointsForBackend,
		func(a, b EndpointsForBackend) bool { return a.Equals(b) },
		cases,
		[]string{
			// Folded into upstreamHash by NewEndpointsForBackend; see
			// TestEndpointsForBackendEqualityHashCoversLabels.
			"BackendLabels",
			// Folded into LbEpsEqualityHash by newFinalBackendEndpoints; see
			// TestFinalBackendEndpointsEqualsObservesPolicyChange in proxy_syncer.
			"AttachedPolicies",
		},
	)
}

// TestEndpointsForBackendEmptyCopyEquality documents that EmptyCopy drops the
// endpoints (and so must not compare equal to a populated set) while preserving
// the backend identity hash.
func TestEndpointsForBackendEmptyCopyEquality(t *testing.T) {
	populated := baseHarnessEndpointsForBackend()

	empty := populated.EmptyCopy()
	if populated.Equals(empty) {
		t.Error("Equals returned true comparing a populated EndpointsForBackend to its EmptyCopy")
	}

	// Two EmptyCopies of the same backend must still be equal.
	if !empty.Equals(populated.EmptyCopy()) {
		t.Error("two EmptyCopy results for the same backend compared unequal")
	}
}
