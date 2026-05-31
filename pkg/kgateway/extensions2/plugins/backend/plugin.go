package backend

import (
	"context"
	"errors"
	"fmt"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoywellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

var logger = logging.New("plugin/backend")

const (
	ExtensionName = "backend"
)

var errAwsEc2DiscoveryDisabled = errors.New("aws ec2 discovery is disabled by controller settings")

// backendIr is the internal representation of a backend.
type backendIr struct {
	awsIr    *AwsIr
	staticIr *StaticIr
	dfpIr    *DfpIr
	gcpIr    *GcpIr
	errors   []error
}

func (u *backendIr) Equals(other any) bool {
	otherBackend, ok := other.(*backendIr)
	if !ok {
		return false
	}
	// AWS
	if !u.awsIr.Equals(otherBackend.awsIr) {
		return false
	}
	// Static
	if !u.staticIr.Equals(otherBackend.staticIr) {
		return false
	}
	// DFP
	if !u.dfpIr.Equals(otherBackend.dfpIr) {
		return false
	}
	// GCP
	if !u.gcpIr.Equals(otherBackend.gcpIr) {
		return false
	}
	if len(u.errors) != len(otherBackend.errors) {
		return false
	}
	for i := range u.errors {
		if !backendIRErrorEqual(u.errors[i], otherBackend.errors[i]) {
			return false
		}
	}
	return true
}

func backendIRErrorEqual(a, b error) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Error() == b.Error()
	}
}

func NewPlugin(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin {
	cli := kclient.NewFilteredDelayed[*kgateway.Backend](
		commoncol.Client,
		wellknown.BackendGVR,
		kclient.Filter{ObjectFilter: commoncol.Client.ObjectFilter()},
	)

	col := krt.WrapClient(cli, commoncol.KrtOpts.ToOptions("Backends")...)

	gk := wellknown.BackendGVK.GroupKind()
	translateFn := buildTranslateFunc(commoncol.Secrets, commoncol.Settings.EnableAwsEc2Discovery)
	bcol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *kgateway.Backend) *ir.BackendObjectIR {
		backendIR := translateFn(krtctx, i)
		if len(backendIR.errors) > 0 {
			logger.Error("failed to translate backend", "backend", i.GetName(), "error", errors.Join(backendIR.errors...))
		}
		objSrc := ir.ObjectSource{
			Kind:      gk.Kind,
			Group:     gk.Group,
			Namespace: i.GetNamespace(),
			Name:      i.GetName(),
		}
		backend := ir.NewBackendObjectIR(objSrc, 0, "", ExtensionName)
		backend.CanonicalHostname = hostname(i)
		backend.AppProtocol = parseAppProtocol(i)
		backend.Obj = i
		backend.ObjIr = backendIR
		backend.Errors = backendIR.errors

		// Parse common annotations
		ir.ParseObjectAnnotations(&backend, i)
		return &backend
	})
	ec2Endpoints := newEc2EndpointsCollection(ctx, commoncol, bcol)
	return sdk.Plugin{
		ContributesBackends: map[schema.GroupKind]sdk.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitEnvoyBackend: processBackendForEnvoy,
				},
				Backends:  bcol,
				Endpoints: ec2Endpoints.Endpoints,
			},
		},
		ContributesPolicies: map[schema.GroupKind]sdk.PolicyPlugin{
			wellknown.BackendGVK.GroupKind(): {
				Name:                      "backend",
				NewGatewayTranslationPass: newPlug,
			},
		},
		ContributesLeaderAction: map[schema.GroupKind]func(){
			wellknown.BackendGVK.GroupKind(): buildRegisterCallback(cli, bcol),
		},
		ExtraHasSynced: ec2Endpoints.HasSynced,
	}
}

// buildTranslateFunc builds a function that translates a Backend to a backendIr that
// the plugin can use to build the envoy config.
func buildTranslateFunc(
	secrets *krtcollections.SecretIndex,
	enableAwsEc2Discovery bool,
) func(krtctx krt.HandlerContext, i *kgateway.Backend) *backendIr {
	return func(krtctx krt.HandlerContext, i *kgateway.Backend) *backendIr {
		var beIr backendIr
		switch {
		case i.Spec.Static != nil:
			staticIr, err := buildStaticIr(i.Spec.Static)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}
			beIr.staticIr = staticIr
		case i.Spec.DynamicForwardProxy != nil:
			dfpIr, err := buildDfpIr(i.Spec.DynamicForwardProxy)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}
			beIr.dfpIr = dfpIr
		case i.Spec.Aws != nil:
			switch {
			case i.Spec.Aws.Lambda != nil:
				region := defaultAwsRegion(i.Spec.Aws.Region)
				invokeMode := getLambdaInvocationMode(i.Spec.Aws)

				secret, err := loadAWSSecret(krtctx, secrets, i)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					break
				}

				lambdaArn, err := buildLambdaARN(i.Spec.Aws, region)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					break
				}

				endpointConfig, err := configureLambdaEndpoint(i.Spec.Aws)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					return &beIr
				}

				var lambdaTransportSocket *envoycorev3.TransportSocket
				if endpointConfig.useTLS {
					// TODO(yuval-k): Add verification context
					typedConfig, err := utils.MessageToAny(&envoytlsv3.UpstreamTlsContext{
						Sni: endpointConfig.hostname,
					})
					if err != nil {
						beIr.errors = append(beIr.errors, err)
						break
					}
					lambdaTransportSocket = &envoycorev3.TransportSocket{
						Name: envoywellknown.TransportSocketTls,
						ConfigType: &envoycorev3.TransportSocket_TypedConfig{
							TypedConfig: typedConfig,
						},
					}
				}

				lambdaFilters, err := buildLambdaFilters(
					lambdaArn, region, secret, invokeMode, i.Spec.Aws.Lambda.PayloadTransformMode)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					break
				}

				beIr.awsIr = &AwsIr{
					lambdaIr: &LambdaIr{
						lambdaEndpoint:        endpointConfig,
						lambdaTransportSocket: lambdaTransportSocket,
						lambdaFilters:         lambdaFilters,
					},
				}
			case i.Spec.Aws.Ec2 != nil:
				if !enableAwsEc2Discovery {
					beIr.errors = append(beIr.errors, errAwsEc2DiscoveryDisabled)
					break
				}
				secret, err := loadAWSSecret(krtctx, secrets, i)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					break
				}
				ec2Ir, err := buildEc2Ir(i.Spec.Aws, secret)
				if err != nil {
					beIr.errors = append(beIr.errors, err)
					break
				}
				beIr.awsIr = &AwsIr{ec2Ir: ec2Ir}
			}
		case i.Spec.Gcp != nil:
			gcpIr, err := buildGcpIr(i.Spec.Gcp)
			if err != nil {
				beIr.errors = append(beIr.errors, err)
			}
			beIr.gcpIr = gcpIr
		}
		return &beIr
	}
}

func loadAWSSecret(krtctx krt.HandlerContext, secrets *krtcollections.SecretIndex, backend *kgateway.Backend) (*ir.Secret, error) {
	if backend.Spec.Aws == nil || backend.Spec.Aws.Auth == nil || backend.Spec.Aws.Auth.Type != kgateway.AwsAuthTypeSecret {
		return nil, nil
	}
	if backend.Spec.Aws.Auth.SecretRef == nil {
		return nil, fmt.Errorf("aws auth secretRef is required when type is %q", kgateway.AwsAuthTypeSecret)
	}
	if secrets == nil {
		return nil, errors.New("aws secret lookup is unavailable")
	}

	secretName := backend.Spec.Aws.Auth.SecretRef.Name
	secret, err := secrets.GetSecretWithoutRefGrant(krtctx, secretName, backend.GetNamespace())
	if err != nil {
		logAWSSecretReferenceError(backend, secretName, err)
		return nil, err
	}
	return secret, nil
}

func logAWSSecretReferenceError(backend *kgateway.Backend, secretName string, err error) {
	logger.Error(
		"referenced AWS secret does not exist or could not be loaded",
		"backend", fmt.Sprintf("%s/%s", backend.GetNamespace(), backend.GetName()),
		"secret", fmt.Sprintf("%s/%s", backend.GetNamespace(), secretName),
		"error", err,
	)
}

func processBackendForEnvoy(ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) *ir.EndpointsForBackend {
	be, ok := in.Obj.(*kgateway.Backend)
	if !ok {
		logger.Error("failed to cast backend object")
		return nil
	}
	beIr, ok := in.ObjIr.(*backendIr)
	if !ok {
		logger.Error("failed to cast backend ir")
		return nil
	}

	// TODO: propagated error to CRD #11558.
	spec := be.Spec
	switch {
	case spec.Static != nil:
		processStatic(beIr.staticIr, out)
	case spec.Aws != nil:
		if beIr.awsIr == nil {
			return nil
		}
		if err := processAws(beIr.awsIr, out); err != nil {
			logger.Error("failed to process aws backend", "error", err)
			beIr.errors = append(beIr.errors, err)
		}
	case spec.DynamicForwardProxy != nil:
		processDynamicForwardProxy(beIr.dfpIr, out)
	case spec.Gcp != nil:
		if err := processGcp(beIr.gcpIr, out); err != nil {
			logger.Error("failed to process gcp backend", "error", err)
			beIr.errors = append(beIr.errors, err)
		}
	}
	return nil
}

func parseAppProtocol(b *kgateway.Backend) ir.AppProtocol {
	if b.Spec.Static != nil {
		appProtocol := b.Spec.Static.AppProtocol
		if appProtocol != nil {
			return ir.ParseAppProtocol(new(string(*appProtocol)))
		}
	}
	return ir.DefaultAppProtocol
}

// hostname returns the hostname for the backend. Only static backends are supported.
func hostname(in *kgateway.Backend) string {
	if in.Spec.Static == nil {
		return ""
	}
	if len(in.Spec.Static.Hosts) == 0 {
		return ""
	}
	return in.Spec.Static.Hosts[0].Host
}

type backendPlugin struct {
	ir.UnimplementedProxyTranslationPass
	needsDfpFilter map[string]bool
	needsGcpAuthn  map[string]bool
}

var _ ir.ProxyTranslationPass = &backendPlugin{}

func newPlug(tctx ir.GwTranslationCtx, reporter reporter.Reporter) ir.ProxyTranslationPass {
	return &backendPlugin{}
}

func (p *backendPlugin) Name() string {
	return ExtensionName
}

func (p *backendPlugin) ApplyForBackend(pCtx *ir.RouteBackendContext, in ir.HttpBackend, out *envoyroutev3.Route) error {
	backend := pCtx.Backend.Obj.(*kgateway.Backend)
	if backend.Spec.DynamicForwardProxy != nil {
		if p.needsDfpFilter == nil {
			p.needsDfpFilter = make(map[string]bool)
		}
		p.needsDfpFilter[pCtx.FilterChainName] = true
	}

	if backend.Spec.Gcp != nil {
		if p.needsGcpAuthn == nil {
			p.needsGcpAuthn = make(map[string]bool)
		}
		p.needsGcpAuthn[pCtx.FilterChainName] = true

		// Set host rewrite for GCP backends (only if not already set by another policy)
		routeAction := out.GetRoute()
		if routeAction == nil {
			routeAction = &envoyroutev3.RouteAction{}
			out.Action = &envoyroutev3.Route_Route{
				Route: routeAction,
			}
		}
		// Set auto host rewrite if not already configured
		if routeAction.GetHostRewriteSpecifier() == nil {
			routeAction.HostRewriteSpecifier = &envoyroutev3.RouteAction_AutoHostRewrite{
				AutoHostRewrite: &wrapperspb.BoolValue{Value: true},
			}
		}
	}

	return nil
}

// called 1 time per listener
// if a plugin emits new filters, they must be with a plugin unique name.
// any filter returned from route config must be disabled, so it doesnt impact other routes.
func (p *backendPlugin) HttpFilters(_ ir.HttpFiltersContext, fc ir.FilterChainCommon) ([]filters.StagedHttpFilter, error) {
	result := []filters.StagedHttpFilter{}

	var errs []error
	if p.needsDfpFilter[fc.FilterChainName] {
		pluginStage := filters.DuringStage(filters.OutAuthStage)
		f := filters.MustNewStagedFilter("envoy.filters.http.dynamic_forward_proxy", dfpFilterConfig, pluginStage)
		result = append(result, f)
	}
	if p.needsGcpAuthn[fc.FilterChainName] {
		pluginStage := filters.BeforeStage(filters.RouteStage)
		f := filters.MustNewStagedFilter(gcpAuthnFilterName, getGcpAuthnFilterConfig(), pluginStage)
		result = append(result, f)
	}
	return result, errors.Join(errs...)
}

// called 1 time (per envoy proxy). replaces GeneratedResources
func (p *backendPlugin) ResourcesToAdd() ir.Resources {
	resources := ir.Resources{}
	// Add GCP metadata cluster if any GCP backends are present
	if len(p.needsGcpAuthn) > 0 {
		resources.Clusters = []*envoyclusterv3.Cluster{getGcpAuthnCluster()}
	}
	return resources
}
