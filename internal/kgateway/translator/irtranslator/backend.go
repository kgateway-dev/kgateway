package irtranslator

import (
	"context"
	"errors"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/endpoints"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

const clusterConnectionTimeout = time.Second * 5

type BackendTranslator struct {
	ContributedBackends map[schema.GroupKind]ir.BackendInit
	ContributedPolicies map[schema.GroupKind]sdk.PolicyPlugin
	CommonCols          *collections.CommonCollections
	Validator           validator.Validator
	Mode                apisettings.ValidationMode
}

// TranslateBackend translates a BackendObjectIR to an Envoy Cluster. If we encounter any
// errors during translation, a blackhole cluster is returned along with the error. The error
// return value is what matters as consumers (internal/kgateway/proxy_syncer/perclient.go) will
// drop errored clusters from the xDS snapshot and track them separately for status reporting.
// The blackhole cluster itself is not sent to Envoy but provides a consistent return structure.
func (t *BackendTranslator) TranslateBackend(
	ctx context.Context,
	kctx krt.HandlerContext,
	ucc ir.UniqlyConnectedClient,
	backend *ir.BackendObjectIR,
) (*envoyclusterv3.Cluster, error) {
	// defensive checks that the backend is supported and has a plugin that can translate it.
	gk := schema.GroupKind{
		Group: backend.Group,
		Kind:  backend.Kind,
	}
	process, ok := t.ContributedBackends[gk]
	if !ok {
		return nil, errors.New("no backend translator found for " + gk.String())
	}
	if process.InitEnvoyBackend == nil {
		return nil, errors.New("no backend plugin found for " + gk.String())
	}

	// Check for pre-existing errors in the Backend IR before starting translation.
	// Exit translation early if we have errors
	if backend.Errors != nil {
		logger.Error("backend has pre-existing errors", "backend", backend.Name, "errors", backend.Errors)
		return buildBlackholeCluster(backend), errors.Join(backend.Errors...)
	}

	// Initialize the cluster with minimal configuration
	out := initializeCluster(backend)
	inlineEps := process.InitEnvoyBackend(ctx, *backend, out)
	processDnsLookupFamily(out, t.CommonCols)

	// Apply policies to the computed cluster
	if err := t.runPolicies(kctx, ctx, ucc, backend, inlineEps, out); err != nil {
		logger.Error("failed to apply policies to cluster", "cluster", out.GetName(), "error", err)
		return buildBlackholeCluster(backend), err
	}

	// In strict mode, validate the final cluster configuration using Envoy
	if t.Mode == apisettings.ValidationStrict && t.Validator != nil {
		if err := t.validateClusterConfig(ctx, out); err != nil {
			logger.Error("cluster failed xDS validation in strict mode", "cluster", out.GetName(), "error", err)
			return buildBlackholeCluster(backend), err
		}
	}

	return out, nil
}

func (t *BackendTranslator) runPolicies(
	kctx krt.HandlerContext,
	ctx context.Context,
	ucc ir.UniqlyConnectedClient,
	backend *ir.BackendObjectIR,
	inlineEps *ir.EndpointsForBackend,
	out *envoyclusterv3.Cluster,
) error {
	// if the backend was initialized with inlineEps then we
	// need an EndpointsInputs to run plugins against
	var endpointInputs *endpoints.EndpointsInputs
	if inlineEps != nil {
		endpointInputs = &endpoints.EndpointsInputs{
			EndpointsForBackend: *inlineEps,
		}
	}

	var errs []error
	for gk, policyPlugin := range t.ContributedPolicies {
		// TODO: in theory it would be nice to do `ProcessBackend` once, and only do
		// the the per-client processing for each client.
		// that would require refactoring and thinking about the proper IR, so we'll punt on that for
		// now, until we have more backend plugin examples to properly understand what it should look
		// like.
		if policyPlugin.PerClientProcessBackend != nil {
			policyPlugin.PerClientProcessBackend(kctx, ctx, ucc, *backend, out)
		}
		// run endpoint plugins if we have endpoints to process
		if endpointInputs != nil && policyPlugin.PerClientProcessEndpoints != nil {
			policyPlugin.PerClientProcessEndpoints(kctx, ctx, ucc, endpointInputs)
		}
		// if the policy plugin has no ProcessBackend function, skip it
		if policyPlugin.ProcessBackend == nil {
			continue
		}
		// apply plugins to the backend. we want to skip applying the plugin if the
		// attached IR encountered any errors during construction.
		for _, polAttachment := range backend.AttachedPolicies.Policies[gk] {
			if len(polAttachment.Errors) > 0 {
				logger.Error("policy has errors", "gk", gk, "errors", polAttachment.Errors, "policyRef", polAttachment.PolicyRef)
				errs = append(errs, polAttachment.Errors...)
				continue
			}
			policyPlugin.ProcessBackend(ctx, polAttachment.PolicyIr, *backend, out)
		}
	}

	// for clusters that want a CLA _and_ initialized with inlineEps, build the CLA.
	// never overwrite the CLA that was already initialized (potentially within a plugin).
	if out.GetLoadAssignment() == nil && endpointInputs != nil && clusterSupportsInlineCLA(out) {
		out.LoadAssignment = endpoints.PrioritizeEndpoints(
			logger,
			ucc,
			*endpointInputs,
		)
	}

	return errors.Join(errs...)
}

// validateClusterConfig validates an individual cluster configuration using Envoy's
// validation. This catches configuration errors that would cause Envoy data plane NACKs,
// such as invalid cipher suites, invalid TLS parameters, etc.
func (t *BackendTranslator) validateClusterConfig(ctx context.Context, cluster *envoyclusterv3.Cluster) error {
	builder := bootstrap.New()
	builder.AddCluster(cluster)
	bootstrap, err := builder.Build()
	if err != nil {
		return err
	}
	data, err := protojson.Marshal(bootstrap)
	if err != nil {
		return err
	}
	if err := t.Validator.Validate(ctx, string(data)); err != nil {
		return err
	}
	return nil
}

var inlineCLAClusterTypes = sets.New(
	envoyclusterv3.Cluster_STATIC,
	envoyclusterv3.Cluster_STRICT_DNS,
	envoyclusterv3.Cluster_LOGICAL_DNS,
)

func clusterSupportsInlineCLA(cluster *envoyclusterv3.Cluster) bool {
	return inlineCLAClusterTypes.Has(cluster.GetType())
}

var h2Options = func() *anypb.Any {
	http2ProtocolOptions := &envoy_upstreams_v3.HttpProtocolOptions{
		UpstreamProtocolOptions: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
				},
			},
		},
	}

	a, err := utils.MessageToAny(http2ProtocolOptions)
	if err != nil {
		// should never happen - all values are known ahead of time.
		panic(err)
	}
	return a
}()

// processDnsLookupFamily modifies clusters that use DNS-based discovery in the following way:
// 1. explicitly default to 'V4_PREFERRED' (as opposed to the envoy default of effectively V6_PREFERRED)
// 2. override to value defined in kgateway global setting if present
func processDnsLookupFamily(out *envoyclusterv3.Cluster, cc *collections.CommonCollections) {
	cdt, ok := out.GetClusterDiscoveryType().(*envoyclusterv3.Cluster_Type)
	if !ok {
		return
	}
	setDns := false
	switch cdt.Type {
	case envoyclusterv3.Cluster_STATIC, envoyclusterv3.Cluster_LOGICAL_DNS, envoyclusterv3.Cluster_STRICT_DNS:
		setDns = true
	}
	if !setDns {
		return
	}

	// irrespective of settings, default to V4_PREFERRED, overriding Envoy default
	out.DnsLookupFamily = envoyclusterv3.Cluster_V4_PREFERRED

	if cc == nil {
		return
	}
	// if we have settings, use value from it
	switch cc.Settings.DnsLookupFamily {
	case apisettings.DnsLookupFamilyV4Preferred:
		out.DnsLookupFamily = envoyclusterv3.Cluster_V4_PREFERRED
	case apisettings.DnsLookupFamilyV4Only:
		out.DnsLookupFamily = envoyclusterv3.Cluster_V4_ONLY
	case apisettings.DnsLookupFamilyV6Only:
		out.DnsLookupFamily = envoyclusterv3.Cluster_V6_ONLY
	case apisettings.DnsLookupFamilyAuto:
		out.DnsLookupFamily = envoyclusterv3.Cluster_AUTO
	case apisettings.DnsLookupFamilyAll:
		out.DnsLookupFamily = envoyclusterv3.Cluster_ALL
	}
}

func translateAppProtocol(appProtocol ir.AppProtocol) map[string]*anypb.Any {
	typedExtensionProtocolOptions := map[string]*anypb.Any{}
	switch appProtocol {
	case ir.HTTP2AppProtocol:
		typedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] = proto.Clone(h2Options).(*anypb.Any)
	}
	return typedExtensionProtocolOptions
}

// initializeCluster creates a default envoy cluster with minimal configuration,
// that will then be augmented by various backend plugins
func initializeCluster(b *ir.BackendObjectIR) *envoyclusterv3.Cluster {
	out := &envoyclusterv3.Cluster{
		Name:                          b.ClusterName(),
		Metadata:                      new(envoycorev3.Metadata),
		ConnectTimeout:                durationpb.New(clusterConnectionTimeout),
		TypedExtensionProtocolOptions: translateAppProtocol(b.AppProtocol),
		CommonLbConfig:                createCommonLbConfig(b),
	}
	return out
}

func buildBlackholeCluster(b *ir.BackendObjectIR) *envoyclusterv3.Cluster {
	out := &envoyclusterv3.Cluster{
		Name:     b.ClusterName(),
		Metadata: new(envoycorev3.Metadata),
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: envoyclusterv3.Cluster_STATIC,
		},
		LoadAssignment: &envoyendpointv3.ClusterLoadAssignment{
			ClusterName: b.ClusterName(),
			Endpoints:   []*envoyendpointv3.LocalityLbEndpoints{},
		},
	}
	return out
}

func createCommonLbConfig(b *ir.BackendObjectIR) *envoyclusterv3.Cluster_CommonLbConfig {
	if b.TrafficDistribution != wellknown.TrafficDistributionAny {
		return &envoyclusterv3.Cluster_CommonLbConfig{
			LocalityConfigSpecifier: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &envoyclusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			},
		}
	}
	return nil
}
