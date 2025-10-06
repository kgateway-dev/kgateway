package main

import (
	"context"
	"time"

	mutation_v3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/setup"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	collections "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

/******

An example plugin, that uses a ConfigMap as a policy. We use a targetRef annotation to attach the
policy to an HTTPRouter. We will then add the key value pairs in the ConfigMap to the metadata of
the envoy route route.

Example ConfigMap:

apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
  annotations:
    targetRef: my-http-route
data:
  key1: value1
  key2: value2


To test, use this example HTTPRoute that adds the metadata to a header:

apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-http-route
spec:
  parentRefs:
  - name: gw
  rules:
  - backendRefs:
    - name: example-svc
      port: 8080
    filters:
    - type: ResponseHeaderModifier
      responseHeaderModifier:
        add:
        - name: my-header-name
          value: %METADATA(ROUTE:example.plugin:key1)%

*****/

var configMapGK = schema.GroupKind{
	Group: "",
	Kind:  "ConfigMap",
}

// Our policy IR.
type configMapIr struct {
	creationTime time.Time
	metadata     *structpb.Struct
}

var _ ir.PolicyIR = &configMapIr{}

// in case multiple policies attached to the same resource, we sort by policy creation time.
func (d *configMapIr) CreationTime() time.Time {
	return d.creationTime
}

// Equals is needed because this is in KRT collection.
func (d *configMapIr) Equals(in any) bool {
	d2, ok := in.(*configMapIr)
	if !ok {
		return false
	}
	return d.creationTime == d2.creationTime && proto.Equal(d.metadata, d2.metadata)
}

// convert a configmap to our IR.
func configMapToIr(cm *corev1.ConfigMap) *configMapIr {
	// When converting to IR, we want to take the translation as close to envoy xDS as we can.
	// That's why our IR intentionaly uses structpb.Struct and not map[string]string.

	mdStruct := &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}
	for k, v := range cm.Data {
		mdStruct.Fields[k] = structpb.NewStringValue(v)
	}

	return &configMapIr{
		creationTime: cm.CreationTimestamp.Time,
		metadata:     mdStruct,
	}
}

// Configmaps don't have a target ref, so we extract it from the annotations.
func extractTargetRefs(cm *corev1.ConfigMap) []ir.PolicyRef {
	return []ir.PolicyRef{{
		Group: gwv1.GroupName,
		Kind:  "HTTPRoute",
		Name:  cm.Annotations["targetRef"],
	}}
}

// Create a collection of our policies. This will be done by converting a configmap collection
// to our policy IR.
func ourPolicies(commoncol *collections.CommonCollections) krt.Collection[ir.PolicyWrapper] {
	// We create 2 collections here - one for the source config maps, and one for the policy IR.
	// Whenever creating a new krtCollection use commoncol.KrtOpts.ToOptions("<Name>") to provide the
	// collection with common options and a name. It's important so that the collection appears in
	// the krt debug page.

	// Use the default discovery namespace filter to filter configmaps not in the list of discovery namespaces
	filter := kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()}

	// get a configmap client going
	configMapCol := krt.WrapClient(kclient.NewFiltered[*corev1.ConfigMap](commoncol.Client, filter), commoncol.KrtOpts.ToOptions("ConfigMaps")...)

	// convertIt to policy IR
	return krt.NewCollection(configMapCol, func(krtctx krt.HandlerContext, i *corev1.ConfigMap) *ir.PolicyWrapper {
		if i.Annotations["targetRef"] == "" {
			return nil
		}

		pol := &ir.PolicyWrapper{
			ObjectSource: ir.ObjectSource{
				Group:     configMapGK.Group,
				Kind:      configMapGK.Kind,
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Policy:     i,
			PolicyIR:   configMapToIr(i),
			TargetRefs: extractTargetRefs(i),
		}
		return pol
	}, commoncol.KrtOpts.ToOptions("MetadataPolicies")...)
}

// Our translation pass struct. This holds translation specific state.
// In our case, we check if our policy was applied to a route and if so, we add a filter.
type ourPolicyPass struct {
	// Add the unimplemented pass so we don't have to implement all the methods.
	ir.UnimplementedProxyTranslationPass

	// We keep track of which filter chains need our filter.
	filterNeeded map[string]bool
}

// ApplyForRoute is called when a an HTTPRouteRule is being translated to an envoy route.
func (s *ourPolicyPass) ApplyForRoute(pCtx *ir.RouteContext, out *envoyroutev3.Route) error {
	// get our policy IR. Kgateway used the targetRef to attach the policy to the HTTPRoute. and now as it
	// translates the HTTPRoute to xDS, it calls our plugin and passes the policy for the plugin's translation pass to do the
	// policy to xDS translation.
	cmIr, ok := pCtx.Policy.(*configMapIr)
	if !ok {
		// should never happen
		return nil
	}
	// apply the metadata from our IR to envoy's route object
	if out.Metadata == nil {
		out.Metadata = &envoycorev3.Metadata{}
	}
	out.Metadata.FilterMetadata["example.plugin"] = cmIr.metadata

	// mark that we need a filter for this filter chain.
	if s.filterNeeded == nil {
		s.filterNeeded = map[string]bool{}
	}
	s.filterNeeded[pCtx.FilterChainName] = true

	return nil
}

func (s *ourPolicyPass) HttpFilters(fc ir.FilterChainCommon) ([]plugins.StagedHttpFilter, error) {
	if !s.filterNeeded[fc.FilterChainName] {
		return nil, nil
	}
	// Add an http filter to the chain that adds a header indicating metadata was added.
	return []plugins.StagedHttpFilter{
		plugins.MustNewStagedFilter("example_plugin",
			&header_mutationv3.HeaderMutation{
				Mutations: &header_mutationv3.Mutations{
					ResponseMutations: []*mutation_v3.HeaderMutation{
						{
							Action: &mutation_v3.HeaderMutation_Append{
								Append: &envoycorev3.HeaderValueOption{
									Header: &envoycorev3.HeaderValue{
										Key:   "x-metadata-added",
										Value: "true",
									},
								},
							},
						},
					},
				},
			},
			plugins.BeforeStage(plugins.AcceptedStage)),
	}, nil
}

// A function that initializes our plugins.
func pluginFactory(ctx context.Context, commoncol *collections.CommonCollections, mergeSettingsJSON string) []sdk.Plugin {
	return []sdk.Plugin{
		{
			ContributesPolicies: sdk.ContributesPolicies{
				configMapGK: sdk.PolicyPlugin{
					Name: "metadataPolicy",
					NewGatewayTranslationPass: func(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
						// Return a fresh new translation pass
						return &ourPolicyPass{}
					},
					// Provide a collection of our policies in IR form.
					Policies: ourPolicies(commoncol),
				},
			},
		},
	}
}

func extraGatewayParametersFactory(cli client.Client, inputs *deployer.Inputs) []deployer.ExtraGatewayParameters {
	return make([]deployer.ExtraGatewayParameters, 0)
}

func main() {
	// TODO: move setup.StartGGv2 from internal to public.
	// Start Kgateway and provide our plugin.
	// This demonstrates how to start Kgateway with a custom plugin.
	// This binary is the control plane. normally it would be packaged in a docker image and run
	// in a k8s cluster.

	setup, _ := setup.New(
		setup.WithExtraPlugins(pluginFactory),
		setup.ExtraGatewayParameters(extraGatewayParametersFactory),
	)
	setup.Start(context.Background())
}
