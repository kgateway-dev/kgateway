package trafficpolicy

import (
	"context"
	"errors"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// overrideGK stands in for a kind other than TrafficPolicy that carries the spec being
// translated, and so holds the references a ReferenceGrant has to name.
var overrideGK = schema.GroupKind{Group: "example.io", Kind: "ExamplePolicy"}

// gatewayExtensionRefGrant builds a ReferenceGrant in ext-ns permitting references to
// GatewayExtensions from fromGK in app-ns.
func gatewayExtensionRefGrant(fromGK schema.GroupKind) *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "ext-ns"},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{{
				Group:     gwv1.Group(fromGK.Group),
				Kind:      gwv1.Kind(fromGK.Kind),
				Namespace: "app-ns",
			}},
			To: []gwv1b1.ReferenceGrantTo{{
				Group: gwv1.Group(wellknown.GatewayExtensionGVK.Group),
				Kind:  gwv1.Kind(wellknown.GatewayExtensionGVK.Kind),
			}},
		},
	}
}

// TestFetchGatewayExtensionRefGrantSourceIdentity covers a strict-mode cross-namespace
// extensionRef. The source identity is exactly one kind, so overriding it moves which
// grant applies rather than widening the set. Passing the grant check is observable as
// the ExtensionRef then failing to resolve, since no GatewayExtension exists.
func TestFetchGatewayExtensionRefGrantSourceIdentity(t *testing.T) {
	tests := []struct {
		name    string
		grantGK schema.GroupKind
		opts    []TrafficPolicyConstructorOption
		allowed bool
	}{
		{
			name:    "default identity, grant names TrafficPolicy",
			grantGK: wellknown.TrafficPolicyGVK.GroupKind(),
			allowed: true,
		},
		{
			name:    "default identity, grant names another kind",
			grantGK: overrideGK,
		},
		{
			name:    "overridden identity, grant names that kind",
			grantGK: overrideGK,
			opts:    []TrafficPolicyConstructorOption{WithSourceGroupKind(overrideGK)},
			allowed: true,
		},
		{
			// The override replaces the default: a namespace that granted access to
			// TrafficPolicy has not granted it to the overriding kind.
			name:    "overridden identity, grant names TrafficPolicy",
			grantGK: wellknown.TrafficPolicyGVK.GroupKind(),
			opts:    []TrafficPolicyConstructorOption{WithSourceGroupKind(overrideGK)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
			grants := krt.NewStaticCollection[*gwv1b1.ReferenceGrant](nil,
				[]*gwv1b1.ReferenceGrant{gatewayExtensionRefGrant(tt.grantGK)}, krtopts.ToOptions("RefGrants")...)
			commoncol := &collections.CommonCollections{
				Settings:          apisettings.Settings{ReferenceGrantMode: apisettings.ReferenceGrantStrict},
				RefGrants:         krtcollections.NewRefGrantIndex(grants, apisettings.ReferenceGrantStrict),
				GatewayExtensions: krt.NewStaticCollection[ir.GatewayExtension](nil, nil, krtopts.ToOptions("GatewayExtensions")...),
			}

			c := NewTrafficPolicyConstructor(ctx, commoncol, tt.opts...)
			_, err := c.FetchGatewayExtension(krt.TestingDummyContext{},
				shared.NamespacedObjectReference{Name: "ext", Namespace: ptr.To(gwv1.Namespace("ext-ns"))}, "app-ns")

			switch {
			case tt.allowed && !errors.Is(err, pluginutils.ErrGatewayExtensionNotFound):
				t.Fatalf("FetchGatewayExtension() = %v, want the reference to clear the grant check", err)
			case !tt.allowed && !errors.Is(err, krtcollections.ErrMissingReferenceGrant):
				t.Fatalf("FetchGatewayExtension() = %v, want a missing reference grant error", err)
			}
		})
	}
}

func TestRefGrantSource(t *testing.T) {
	ctx := context.Background()
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	commoncol := &collections.CommonCollections{
		GatewayExtensions: krt.NewStaticCollection[ir.GatewayExtension](nil, nil, krtopts.ToOptions("GatewayExtensions")...),
	}

	from := NewTrafficPolicyConstructor(ctx, commoncol).refGrantSource("app-ns")
	if want := wellknown.TrafficPolicyGVK.GroupKind(); from.GroupKind != want {
		t.Errorf("refGrantSource().GroupKind = %v, want %v", from.GroupKind, want)
	}
	if from.Namespace != "app-ns" {
		t.Errorf("refGrantSource().Namespace = %q, want %q", from.Namespace, "app-ns")
	}

	from = NewTrafficPolicyConstructor(ctx, commoncol, WithSourceGroupKind(overrideGK)).refGrantSource("app-ns")
	if from.GroupKind != overrideGK {
		t.Errorf("refGrantSource().GroupKind = %v, want %v", from.GroupKind, overrideGK)
	}
}
