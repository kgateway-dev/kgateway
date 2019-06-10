package aws

import (
	"context"
	"fmt"
	"net/url"
	"unicode/utf8"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	multierror "github.com/hashicorp/go-multierror"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/transformation"

	"github.com/gogo/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
)

//go:generate protoc -I$GOPATH/src/github.com/envoyproxy/protoc-gen-validate -I. -I$GOPATH/src/github.com/gogo/protobuf/protobuf --gogo_out=Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types:${GOPATH}/src/ filter.proto

const (
	// filter info
	filterName  = "io.solo.aws_lambda"
	pluginStage = plugins.OutAuth

	// cluster info
	accessKey = "access_key"
	secretKey = "secret_key"
)

func getLambdaHostname(s *aws.UpstreamSpec) string {
	return fmt.Sprintf("lambda.%s.amazonaws.com", s.Region)
}

func NewPlugin(transformsAdded *bool) plugins.Plugin {
	return &plugin{
		transformsAdded: transformsAdded}
}

type plugin struct {
	recordedUpstreams map[core.ResourceRef]*aws.UpstreamSpec
	ctx               context.Context
	transformsAdded   *bool
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[core.ResourceRef]*aws.UpstreamSpec)
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	upstreamSpec, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Aws)
	if !ok {
		// not ours
		return nil
	}
	// even if it failed, route should still be valid
	p.recordedUpstreams[in.Metadata.Ref()] = upstreamSpec.Aws

	lambdaHostname := getLambdaHostname(upstreamSpec.Aws)

	// configure Envoy cluster routing info
	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_LOGICAL_DNS,
	}
	// TODO(yuval-k): why do we need to make sure we use ipv4 only dns?
	out.DnsLookupFamily = envoyapi.Cluster_V4_ONLY
	pluginutils.EnvoySingleEndpointLoadAssignment(out, lambdaHostname, 443)

	out.TlsContext = &envoyauth.UpstreamTlsContext{
		// TODO(yuval-k): Add verification context
		Sni: lambdaHostname,
	}

	// TODO(ilacakrms): consider if secretRef should be namespace+name
	secrets, err := params.Snapshot.Secrets.Find(upstreamSpec.Aws.SecretRef.Strings())
	if err != nil {
		return errors.Wrapf(err, "retrieving aws secret")
	}

	awsSecrets, ok := secrets.Kind.(*v1.Secret_Aws)
	if !ok {
		return errors.Errorf("secret %v is not an AWS secret", secrets.GetMetadata().Ref())
	}

	var secretErrs error

	accessKey := awsSecrets.Aws.AccessKey
	secretKey := awsSecrets.Aws.SecretKey
	if accessKey == "" || !utf8.Valid([]byte(accessKey)) {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("access_key is not a valid string"))
	}
	if secretKey == "" || !utf8.Valid([]byte(secretKey)) {
		secretErrs = multierror.Append(secretErrs, errors.Errorf("secret_key is not a valid string"))
	}

	if secretErrs != nil {
		return secretErrs
	}

	lpe := &LambdaProtocolExtension{
		Host:      lambdaHostname,
		Region:    upstreamSpec.Aws.Region,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}

	err = pluginutils.SetExtenstionProtocolOptions(out, filterName, lpe)
	if err != nil {
		return errors.Wrapf(err, "converting aws protocol options to struct")
	}

	return nil
}

func (p *plugin) ProcessRoute(params plugins.Params, in *v1.Route, out *envoyroute.Route) error {
	err := pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, filterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's aws destination
		if spec.DestinationSpec == nil || spec.GetUpstream() == nil {
			return nil, nil
		}
		awsDestinationSpec, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Aws)
		if !ok {
			return nil, nil
		}
		// get upstream
		lambdaSpec, ok := p.recordedUpstreams[*spec.GetUpstream()]
		if !ok {
			err := errors.Errorf("%v is not an AWS upstream", *spec.GetUpstream())
			contextutils.LoggerFrom(p.ctx).Error(err)
			return nil, err
		}
		// should be aws upstream

		// get function
		logicalName := awsDestinationSpec.Aws.LogicalName
		for _, lambdaFunc := range lambdaSpec.LambdaFunctions {
			if lambdaFunc.LogicalName == logicalName {

				lambdaRouteFunc := &LambdaPerRoute{
					Async: awsDestinationSpec.Aws.InvocationStyle == aws.DestinationSpec_ASYNC,
					// we need to query escape per AWS spec:
					// see the CanonicalQueryString section in here: https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
					Qualifier: url.QueryEscape(lambdaFunc.Qualifier),
					Name:      lambdaFunc.LambdaFunctionName,
				}

				return lambdaRouteFunc, nil
			}
		}
		return nil, errors.Errorf("unknown function %v", logicalName)
	})

	if err != nil {
		return err
	}
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's aws destination
		if spec.DestinationSpec == nil {
			return nil, nil
		}
		awsDestinationSpec, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Aws)
		if !ok {
			return nil, nil
		}

		repsonsetransform := awsDestinationSpec.Aws.ResponseTransformation
		if !repsonsetransform {
			return nil, nil
		}
		*p.transformsAdded = true
		return &envoy_transform.RouteTransformations{
			ResponseTransformation: &envoy_transform.Transformation{
				TransformationType: &envoy_transform.Transformation_TransformationTemplate{
					TransformationTemplate: &envoy_transform.TransformationTemplate{
						BodyTransformation: &envoy_transform.TransformationTemplate_Body{
							Body: &envoy_transform.InjaTemplate{
								Text: "{{body}}",
							},
						},
						Headers: map[string]*envoy_transform.InjaTemplate{
							"content-type": {
								Text: "text/html",
							},
						},
					},
				},
			},
		}, nil
	})
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if len(p.recordedUpstreams) == 0 {
		// no upstreams no filter
		return nil, nil
	}
	return []plugins.StagedHttpFilter{
		plugins.NewStagedFilter(filterName, pluginStage),
	}, nil
}
