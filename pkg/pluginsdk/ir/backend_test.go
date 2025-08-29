package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestParseAppProtocol(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected AppProtocol
	}{
		{
			name:     "http2",
			input:    ptr.To("http2"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc",
			input:    ptr.To("grpc"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "grpc-web",
			input:    ptr.To("grpc-web"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/h2c",
			input:    ptr.To("kubernetes.io/h2c"),
			expected: HTTP2AppProtocol,
		},
		{
			name:     "kubernetes.io/ws",
			input:    ptr.To("kubernetes.io/ws"),
			expected: WebSocketAppProtocol,
		},
		{
			name:     "(empty)",
			input:    nil,
			expected: DefaultAppProtocol,
		},
		{
			name:     "unknown",
			input:    ptr.To("unknown"),
			expected: DefaultAppProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			actual := ParseAppProtocol(tt.input)
			a.Equal(tt.expected, actual)
		})
	}
}

// Helper to create a simple test object for testing
func testObject(name, namespace string, uid types.UID, rv string, gen int64) metav1.Object {
	return &metav1.ObjectMeta{
		Name:            name,
		Namespace:       namespace,
		UID:             uid,
		ResourceVersion: rv,
		Generation:      gen,
	}
}

// Helper to create a simple test IR object for testing
func testObjIr() interface{ Equals(any) bool } {
	return &testObjIrImpl{}
}

type testObjIrImpl struct{}

func (t *testObjIrImpl) Equals(other any) bool {
	_, ok := other.(*testObjIrImpl)
	return ok
}

func TestBackendObjectIREquals(t *testing.T) {
	tests := []struct {
		name     string
		backend1 BackendObjectIR
		backend2 BackendObjectIR
		expected bool
	}{
		{
			name: "identical instances are equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{
					Group:     "test.group",
					Kind:      "TestKind",
					Namespace: "test-ns",
					Name:      "test-name",
				},
				Port:                8080,
				AppProtocol:         HTTP2AppProtocol,
				GvPrefix:            "test",
				CanonicalHostname:   "test.example.com",
				ExtraKey:            "extra",
				TrafficDistribution: wellknown.TrafficDistributionAny,
				Obj:                 testObject("test-name", "test-ns", "uid1", "rv1", 1),
				ObjIr:               testObjIr(),
				Aliases: []ObjectSource{
					{Name: "alias1", Namespace: "test-ns"},
					{Name: "alias2", Namespace: "test-ns"},
				},
				AttachedPolicies: AttachedPolicies{},
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{
					Group:     "test.group",
					Kind:      "TestKind",
					Namespace: "test-ns",
					Name:      "test-name",
				},
				Port:                8080,
				AppProtocol:         HTTP2AppProtocol,
				GvPrefix:            "test",
				CanonicalHostname:   "test.example.com",
				ExtraKey:            "extra",
				TrafficDistribution: wellknown.TrafficDistributionAny,
				Obj:                 testObject("test-name", "test-ns", "uid1", "rv1", 1),
				ObjIr:               testObjIr(),
				Aliases: []ObjectSource{
					{Name: "alias1", Namespace: "test-ns"},
					{Name: "alias2", Namespace: "test-ns"},
				},
				AttachedPolicies: AttachedPolicies{},
			},
			expected: true,
		},
		{
			name: "different ObjectSource are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{
					Group:     "test.group",
					Kind:      "TestKind",
					Namespace: "test-ns",
					Name:      "test-name",
				},
				Obj: testObject("test-name", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{
					Group:     "test.group",
					Kind:      "TestKind",
					Namespace: "test-ns",
					Name:      "different-name",
				},
				Obj: testObject("different-name", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different Port are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Port:         8080,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Port:         9090,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different ExtraKey are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ExtraKey:     "key1",
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ExtraKey:     "key2",
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different AppProtocol are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				AppProtocol:  HTTP2AppProtocol,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				AppProtocol:  WebSocketAppProtocol,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different GvPrefix are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				GvPrefix:     "prefix1",
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				GvPrefix:     "prefix2",
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different CanonicalHostname are not equal",
			backend1: BackendObjectIR{
				ObjectSource:      ObjectSource{Name: "test"},
				CanonicalHostname: "host1.example.com",
				Obj:               testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource:      ObjectSource{Name: "test"},
				CanonicalHostname: "host2.example.com",
				Obj:               testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different TrafficDistribution are not equal",
			backend1: BackendObjectIR{
				ObjectSource:        ObjectSource{Name: "test"},
				TrafficDistribution: wellknown.TrafficDistributionAny,
				Obj:                 testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource:        ObjectSource{Name: "test"},
				TrafficDistribution: wellknown.TrafficDistributionPreferSameZone,
				Obj:                 testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different Aliases are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				Aliases: []ObjectSource{
					{Name: "alias1", Namespace: "test-ns"},
					{Name: "alias2", Namespace: "test-ns"},
				},
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				Aliases: []ObjectSource{
					{Name: "alias1", Namespace: "test-ns"},
					{Name: "alias3", Namespace: "test-ns"},
				},
			},
			expected: false,
		},
		{
			name: "different Aliases order are equal (order insensitive)",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				Aliases: []ObjectSource{
					{Name: "alias1", Namespace: "test-ns"},
					{Name: "alias2", Namespace: "test-ns"},
				},
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				Aliases: []ObjectSource{
					{Name: "alias2", Namespace: "test-ns"},
					{Name: "alias1", Namespace: "test-ns"},
				},
			},
			expected: true,
		},
		{
			name: "different Obj are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid2", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "different ObjIr are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        testObjIr(),
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        &differentObjIrImpl{},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "nil vs non-nil ObjIr are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        nil,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        testObjIr(),
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "non-nil vs nil ObjIr are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        testObjIr(),
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        nil,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: false,
		},
		{
			name: "both nil ObjIr are equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        nil,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				ObjIr:        nil,
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
			},
			expected: true,
		},
		{
			name: "different AttachedPolicies are not equal",
			backend1: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				AttachedPolicies: AttachedPolicies{
					Policies: map[schema.GroupKind][]PolicyAtt{
						{Group: "g1", Kind: "k1"}: {{GroupKind: schema.GroupKind{Group: "g1", Kind: "k1"}}},
					},
				},
			},
			backend2: BackendObjectIR{
				ObjectSource: ObjectSource{Name: "test"},
				Obj:          testObject("test", "test-ns", "uid1", "rv1", 1),
				AttachedPolicies: AttachedPolicies{
					Policies: map[schema.GroupKind][]PolicyAtt{
						{Group: "g2", Kind: "k2"}: {{GroupKind: schema.GroupKind{Group: "g2", Kind: "k2"}}},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.backend1.Equals(tt.backend2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry
			reverseResult := tt.backend2.Equals(tt.backend1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}
}

type differentObjIrImpl struct{}

func (t *differentObjIrImpl) Equals(other any) bool {
	_, ok := other.(*differentObjIrImpl)
	return ok
}

func TestAliasesEqual(t *testing.T) {
	tests := []struct {
		name     string
		aliases1 []ObjectSource
		aliases2 []ObjectSource
		expected bool
	}{
		{
			name:     "empty slices are equal",
			aliases1: []ObjectSource{},
			aliases2: []ObjectSource{},
			expected: true,
		},
		{
			name: "identical slices are equal",
			aliases1: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias2", Namespace: "ns2"},
			},
			aliases2: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias2", Namespace: "ns2"},
			},
			expected: true,
		},
		{
			name: "different order but same content are equal",
			aliases1: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias2", Namespace: "ns2"},
			},
			aliases2: []ObjectSource{
				{Name: "alias2", Namespace: "ns2"},
				{Name: "alias1", Namespace: "ns1"},
			},
			expected: true,
		},
		{
			name: "different lengths are not equal",
			aliases1: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
			},
			aliases2: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias2", Namespace: "ns2"},
			},
			expected: false,
		},
		{
			name: "different content are not equal",
			aliases1: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias2", Namespace: "ns2"},
			},
			aliases2: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias3", Namespace: "ns2"},
			},
			expected: false,
		},
		{
			name: "duplicate aliases are handled correctly",
			aliases1: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias1", Namespace: "ns1"},
			},
			aliases2: []ObjectSource{
				{Name: "alias1", Namespace: "ns1"},
				{Name: "alias1", Namespace: "ns1"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aliasesEqual(tt.aliases1, tt.aliases2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry
			reverseResult := aliasesEqual(tt.aliases2, tt.aliases1)
			assert.Equal(t, result, reverseResult, "aliasesEqual should be symmetric")
		})
	}
}
