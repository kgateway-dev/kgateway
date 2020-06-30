package grpc

import (
	"context"
	"crypto/sha1"
	"fmt"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/go-utils/contextutils"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoytranscoder "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/transcoder/v2"
	"github.com/gogo/googleapis/google/api"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"

	"encoding/base64"

	"github.com/gogo/protobuf/types"
	errors "github.com/rotisserie/eris"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooplugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	grpcapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	transformapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	transformutils "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/transformation"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type ServicesAndDescriptor struct {
	Spec        *grpcapi.ServiceSpec
	Descriptors *descriptor.FileDescriptorSet
}

func NewPlugin(transformsAdded *bool) *plugin {
	return &plugin{
		recordedUpstreams: make(map[core.ResourceRef]*v1.Upstream),
		transformsAdded:   transformsAdded,
	}
}

type plugin struct {
	transformsAdded   *bool
	recordedUpstreams map[core.ResourceRef]*v1.Upstream
	upstreamServices  []ServicesAndDescriptor

	ctx context.Context
}

var pluginStage = plugins.BeforeStage(plugins.OutAuthStage)

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	return nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	upstreamType, ok := in.UpstreamType.(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}

	if upstreamType.GetServiceSpec() == nil {
		return nil
	}

	grpcWrapper, ok := upstreamType.GetServiceSpec().PluginType.(*glooplugins.ServiceSpec_Grpc)
	if !ok {
		return nil
	}
	grpcSpec := grpcWrapper.Grpc
	out.Http2ProtocolOptions = &envoycore.Http2ProtocolOptions{}

	if grpcSpec == nil || len(grpcSpec.GrpcServices) == 0 {
		// no services, this just marks the upstream as a grpc one.
		return nil
	}
	descriptors, err := convertProto(grpcSpec.Descriptors)
	if err != nil {
		return errors.Wrapf(err, "parsing grpc spec as a proto descriptor set")
	}

	for _, svc := range grpcSpec.GrpcServices {

		// find the relevant service

		err := addHttpRulesToProto(in, svc, descriptors)
		if err != nil {
			return errors.Wrapf(err, "failed to generate http rules for service %s in proto descriptors", svc.ServiceName)
		}
	}

	addWellKnownProtos(descriptors)

	p.recordedUpstreams[in.Metadata.Ref()] = in
	p.upstreamServices = append(p.upstreamServices, ServicesAndDescriptor{
		Descriptors: descriptors,
		Spec:        grpcSpec,
	})
	contextutils.LoggerFrom(p.ctx).Debugf("in.Metadata.Namespace: %s, in.Metadata.Name: %s", in.Metadata.Namespace, in.Metadata.Name)

	return nil
}

func genFullServiceName(packageName, serviceName string) string {
	if packageName == "" {
		return serviceName
	}
	return packageName + "." + serviceName
}

func convertProto(encodedBytes []byte) (*descriptor.FileDescriptorSet, error) {
	// base-64 encoded by function discovery
	rawDescriptors, err := base64.StdEncoding.DecodeString(string(encodedBytes))
	if err != nil {
		return nil, err
	}
	var fileDescriptor descriptor.FileDescriptorSet
	if err := proto.Unmarshal(rawDescriptors, &fileDescriptor); err != nil {
		return nil, err
	}
	return &fileDescriptor, nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName, func(spec *v1.Destination) (proto.Message, error) {
		// check if it's grpc destination
		if spec.DestinationSpec == nil {
			return nil, nil
		}
		grpcDestinationSpecWrapper, ok := spec.DestinationSpec.DestinationType.(*v1.DestinationSpec_Grpc)
		if !ok {
			return nil, nil
		}
		// copy as it might be modified
		grpcDestinationSpec := *grpcDestinationSpecWrapper.Grpc

		if grpcDestinationSpec.Parameters == nil {
			if out.Match.PathSpecifier == nil {
				return nil, errors.New("missing path for grpc route")
			}
			path := utils.EnvoyPathAsString(out.Match) + "?{query_string}"

			grpcDestinationSpec.Parameters = &transformapi.Parameters{
				Path: &types.StringValue{Value: path},
			}
		}

		// get the package_name.service_name to generate the path that envoy wants
		fullServiceName := genFullServiceName(grpcDestinationSpec.Package, grpcDestinationSpec.Service)
		methodName := grpcDestinationSpec.Function

		upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
		if err != nil {
			contextutils.LoggerFrom(p.ctx).Error(err)
			return nil, err
		}

		upstream := p.recordedUpstreams[*upstreamRef]
		if upstream == nil {
			return nil, errors.New("upstream was not recorded for grpc route")
		}

		// create the transformation for the route
		outPath := httpPath(upstream, fullServiceName, methodName)

		// add query matcher to out path. kombina for now
		// TODO: support query for matching
		outPath += `?{{ default(query_string, "")}}`

		// Add param extractors back
		var extractors map[string]*envoy_transform.Extraction
		if grpcDestinationSpec.Parameters != nil {
			extractors, err = transformutils.CreateRequestExtractors(params.Ctx, grpcDestinationSpec.Parameters)
			if err != nil {
				return nil, err
			}
		}

		// we always choose post
		httpMethod := "POST"
		return &envoy_transform.RouteTransformations{
			RequestTransformation: &envoy_transform.Transformation{
				TransformationType: &envoy_transform.Transformation_TransformationTemplate{
					TransformationTemplate: &envoy_transform.TransformationTemplate{
						Extractors: extractors,
						Headers: map[string]*envoy_transform.InjaTemplate{
							":method": {Text: httpMethod},
							":path":   {Text: outPath},
						},
						BodyTransformation: &envoy_transform.TransformationTemplate_MergeExtractorsToBody{
							MergeExtractorsToBody: &envoy_transform.MergeExtractorsToBody{},
						},
					},
				},
			},
		}, nil
	})
}

// returns package name
func addHttpRulesToProto(upstream *v1.Upstream, currentsvc *grpcapi.ServiceSpec_GrpcService, set *descriptor.FileDescriptorSet) error {
	for _, file := range set.File {
		if file.Package == nil || *file.Package != currentsvc.PackageName {
			continue
		}
		for _, svc := range file.Service {
			if svc.Name == nil || *svc.Name != currentsvc.ServiceName {
				continue
			}
			for _, method := range svc.Method {
				fullServiceName := genFullServiceName(currentsvc.PackageName, currentsvc.ServiceName)
				if method.Options == nil {
					method.Options = &descriptor.MethodOptions{}
				}
				if err := proto.SetExtension(method.Options, api.E_Http, &api.HttpRule{
					Pattern: &api.HttpRule_Post{
						Post: httpPath(upstream, fullServiceName, *method.Name),
					},
					Body: "*",
				}); err != nil {
					return errors.Wrap(err, "setting http extensions for method.Options")
				}
				log.Debugf("method.options: %v", *method.Options)
			}
		}
	}

	return nil
}

func addWellKnownProtos(descriptors *descriptor.FileDescriptorSet) {
	var googleApiHttpFound, googleApiAnnotationsFound, googleApiDescriptorFound bool
	for _, file := range descriptors.File {
		log.Debugf("inspecting descriptor for proto file %v...", *file.Name)
		if *file.Name == "google/api/http.proto" {
			googleApiHttpFound = true
			continue
		}
		if *file.Name == "google/api/annotations.proto" {
			googleApiAnnotationsFound = true
			continue
		}
		if *file.Name == "google/protobuf/descriptor.proto" {
			googleApiDescriptorFound = true
			continue
		}
	}
	if !googleApiDescriptorFound {
		addGoogleApisDescriptor(descriptors)
	}

	if !googleApiHttpFound {
		addGoogleApisHttp(descriptors)
	}

	if !googleApiAnnotationsFound {
		//TODO: investigate if we need this
		//addGoogleApisAnnotations(packageName, set)
	}
}

func httpPath(upstream *v1.Upstream, serviceName, methodName string) string {
	h := sha1.New()
	h.Write([]byte(upstream.Metadata.Namespace + upstream.Metadata.Name + serviceName))
	return "/" + fmt.Sprintf("%x", h.Sum(nil))[:8] + "/" + upstream.Metadata.Name + "/" + serviceName + "/" + methodName
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	if len(p.upstreamServices) == 0 {
		return nil, nil
	}

	var filters []plugins.StagedHttpFilter
	for _, serviceAndDescriptor := range p.upstreamServices {
		descriptorBytes, err := proto.Marshal(serviceAndDescriptor.Descriptors)
		if err != nil {
			return nil, errors.Wrapf(err, "marshaling proto descriptor")
		}
		var fullServiceNames []string
		for _, grpcsvc := range serviceAndDescriptor.Spec.GrpcServices {
			fullName := genFullServiceName(grpcsvc.PackageName, grpcsvc.ServiceName)
			fullServiceNames = append(fullServiceNames, fullName)
		}
		filterConfig := &envoytranscoder.GrpcJsonTranscoder{
			DescriptorSet: &envoytranscoder.GrpcJsonTranscoder_ProtoDescriptorBin{
				ProtoDescriptorBin: descriptorBytes,
			},
			Services:                  fullServiceNames,
			MatchIncomingRequestRoute: true,
		}

		shf, err := plugins.NewStagedFilterWithConfig(wellknown.GRPCJSONTranscoder, filterConfig, pluginStage)
		if err != nil {
			return nil, errors.Wrapf(err, "ERROR: marshaling GrpcJsonTranscoder config")
		}
		filters = append(filters, shf)
	}

	if len(filters) == 0 {
		return nil, errors.Errorf("ERROR: no valid GrpcJsonTranscoder available")
	}

	return filters, nil
}
