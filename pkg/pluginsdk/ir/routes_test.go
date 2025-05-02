package ir

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// makeTCPRoute constructs a TCPRoute with specified metadata.
func makeTCPRoute(name, namespace, rv string, gen int64, uid types.UID) *gwv1alpha2.TCPRoute {
	return &gwv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: rv,
			Generation:      gen,
			UID:             uid,
		},
	}
}

// makeBackendRef constructs a BackendRefIR with no BackendObject for testing.
func makeBackendRef(cluster string, weight uint32) BackendRefIR {
	return BackendRefIR{ClusterName: cluster, Weight: weight, BackendObject: nil}
}

func TestTcpRouteIREquals(t *testing.T) {
	base := makeTCPRoute("route1", "test-ns", "1", 1, types.UID("uid1"))
	same := makeTCPRoute("route1", "test-ns", "1", 1, types.UID("uid1"))

	emptyPolicies := AttachedPolicies{}
	nonEmptyPolicies := AttachedPolicies{
		Policies: map[schema.GroupKind][]PolicyAtt{
			{Group: "g", Kind: "k"}: {{GroupKind: schema.GroupKind{Group: "g", Kind: "k"}}},
		},
	}

	emptyBackends := []BackendRefIR{}
	backendA := []BackendRefIR{makeBackendRef("clusterA", 5)}
	backendB := []BackendRefIR{makeBackendRef("clusterB", 5)}
	backendWeightDiff := []BackendRefIR{makeBackendRef("clusterA", 10)}
	backendErr := []BackendRefIR{{ClusterName: "clusterA", Weight: 5, BackendObject: nil, Err: errors.New("some error")}}

	tests := []struct {
		name string
		a, b TcpRouteIR
		want bool
	}{
		{
			name: "identical_empty",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			want: true,
		},
		{
			name: "diff_objectsource",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route2"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			want: false,
		},
		{
			name: "diff_attached_policies",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: nonEmptyPolicies,
				Backends:         emptyBackends,
			},
			want: false,
		},
		{
			name: "diff_backends_length",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         emptyBackends,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			want: false,
		},
		{
			name: "diff_backend_cluster",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         backendB,
			},
			want: false,
		},
		{
			name: "diff_backend_weight",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         backendWeightDiff,
			},
			want: false,
		},
		{
			name: "identical_backends",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			want: true,
		},
		{
			name: "diff_backend_error",
			a: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     base,
				AttachedPolicies: emptyPolicies,
				Backends:         backendA,
			},
			b: TcpRouteIR{
				ObjectSource:     ObjectSource{Namespace: "test-ns", Name: "route1"},
				SourceObject:     same,
				AttachedPolicies: emptyPolicies,
				Backends:         backendErr,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Equals(tt.b)
			if got != tt.want {
				t.Errorf("TcpRouteIR.Equals() = %v, want %v", got, tt.want)
			}
			// symmetry
			if tt.a.Equals(tt.b) != tt.b.Equals(tt.a) {
				t.Errorf("symmetry mismatch: a.Equals(b)=%v, b.Equals(a)=%v", tt.a.Equals(tt.b), tt.b.Equals(tt.a))
			}
		})
	}
}
