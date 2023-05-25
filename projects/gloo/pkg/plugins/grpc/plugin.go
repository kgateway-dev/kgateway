package grpc

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytranscoder "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_json_transcoder/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/ptypes/wrappers"
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
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"
	"google.golang.org/genproto/googleapis/api/annotations"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.UpstreamPlugin   = new(plugin)
	_ plugins.RoutePlugin      = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName = "grpc"
)

var pluginStage = plugins.BeforeStage(plugins.OutAuthStage)

type plugin struct {
	recordedUpstreams map[string]*v1.Upstream
	upstreamServices  []ServicesAndDescriptor
}

type ServicesAndDescriptor struct {
	Spec        *grpcapi.ServiceSpec
	Descriptors *descriptor.FileDescriptorSet
}

func NewPlugin() *plugin {
	return &plugin{
		recordedUpstreams: make(map[string]*v1.Upstream),
	}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.recordedUpstreams = make(map[string]*v1.Upstream)
	p.upstreamServices = nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	upstreamType, ok := in.GetUpstreamType().(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}

	if upstreamType.GetServiceSpec() == nil {
		return nil
	}
	// If the upstream uses the new API we should record that it exists for use in `ProcessRoute` but not make any changes
	_, ok = upstreamType.GetServiceSpec().GetPluginType().(*glooplugins.ServiceSpec_GrpcJsonTranscoder)
	if ok {
		p.recordedUpstreams[translator.UpstreamToClusterName(in.GetMetadata().Ref())] = in
		return nil
	}
	grpcWrapper, ok := upstreamType.GetServiceSpec().GetPluginType().(*glooplugins.ServiceSpec_Grpc)
	if !ok {
		return nil
	}

	grpcSpec := grpcWrapper.Grpc

	// GRPC transcoding always requires http2
	if out.GetHttp2ProtocolOptions() == nil {
		out.Http2ProtocolOptions = &envoy_config_core_v3.Http2ProtocolOptions{}
	}

	if grpcSpec == nil || len(grpcSpec.GetGrpcServices()) == 0 {
		// no services, this just marks the upstream as a grpc one.
		return nil
	}
	descriptors, err := convertProto(grpcSpec.GetDescriptors())
	if err != nil {
		return errors.Wrapf(err, "parsing grpc spec as a proto descriptor set")
	}

	for _, svc := range grpcSpec.GetGrpcServices() {

		// find the relevant service
		err := addHttpRulesToProto(in, svc, descriptors)
		if err != nil {
			return errors.Wrapf(err, "failed to generate http rules for service %s in proto descriptors", svc.GetServiceName())
		}
	}

	addWellKnownProtos(descriptors)

	p.recordedUpstreams[translator.UpstreamToClusterName(in.GetMetadata().Ref())] = in
	p.upstreamServices = append(p.upstreamServices, ServicesAndDescriptor{
		Descriptors: descriptors,
		Spec:        grpcSpec,
	})
	contextutils.LoggerFrom(params.Ctx).Debugf("record grpc upstream in.Metadata.Namespace: %s, in.Metadata.Name: %s cluster: %s", in.GetMetadata().GetNamespace(), in.GetMetadata().GetName(), translator.UpstreamToClusterName(in.GetMetadata().Ref()))

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

// envoy needs the protobuf descriptors to convert from json to gRPC
// gloo creates these descriptors automatically (if gRPC reflection is enabled),
// uses its transformation filter to provide the context for the json-grpc translation.
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	return pluginutils.MarkPerFilterConfig(params.Ctx, params.Snapshot, in, out, transformation.FilterName,
		func(spec *v1.Destination) (proto.Message, error) {
			// check if it's grpc destination
			if spec.GetDestinationSpec() == nil {
				return nil, nil
			}
			grpcDestinationSpecWrapper, ok := spec.GetDestinationSpec().GetDestinationType().(*v1.DestinationSpec_Grpc)
			if !ok {
				return nil, nil
			}
			// copy as it might be modified
			grpcDestinationSpec := *grpcDestinationSpecWrapper.Grpc

			if grpcDestinationSpec.GetParameters() == nil {
				if out.GetMatch().GetPathSpecifier() == nil {
					return nil, errors.New("missing path for grpc route")
				}
				path := utils.EnvoyPathAsString(out.GetMatch()) + "?{query_string}"

				grpcDestinationSpec.Parameters = &transformapi.Parameters{
					Path: &wrappers.StringValue{Value: path},
				}
			}

			// get the package_name.service_name to generate the path that envoy wants
			fullServiceName := genFullServiceName(grpcDestinationSpec.GetPackage(), grpcDestinationSpec.GetService())
			methodName := grpcDestinationSpec.GetFunction()

			upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
			if err != nil {
				contextutils.LoggerFrom(params.Ctx).Error(err)
				return nil, err
			}

			upstream := p.recordedUpstreams[translator.UpstreamToClusterName(upstreamRef)]
			if upstream == nil {
				return nil, errors.New(fmt.Sprintf("upstream %v was not recorded for grpc route", upstreamRef))
			}
			// If we saved the upstream then it has a ServiceSpec that is either Grpc or GrpcJsonTranscoder
			upstreamType, _ := upstream.GetUpstreamType().(v1.ServiceSpecGetter)

			// If the upstream uses the new API we assume the descriptors and vs match and do not do any transformations
			_, ok = upstreamType.GetServiceSpec().GetPluginType().(*glooplugins.ServiceSpec_GrpcJsonTranscoder)
			if ok {
				return nil, nil
			}
			// create the transformation for the route
			outPath := httpPath(upstream, fullServiceName, methodName)

			// add query matcher to out path. kombina for now
			// TODO: support query for matching
			outPath += `?{{ default(query_string, "")}}`

			// Add param extractors back
			var extractors map[string]*envoy_transform.Extraction
			if grpcDestinationSpec.GetParameters() != nil {
				extractors, err = transformutils.CreateRequestExtractors(params.Ctx, grpcDestinationSpec.GetParameters())
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
		},
	)
}

// returns package name
func addHttpRulesToProto(upstream *v1.Upstream, currentsvc *grpcapi.ServiceSpec_GrpcService, set *descriptor.FileDescriptorSet) error {
	for _, file := range set.GetFile() {
		if file.Package == nil || file.GetPackage() != currentsvc.GetPackageName() {
			continue
		}
		for _, svc := range file.GetService() {
			if svc.Name == nil || svc.GetName() != currentsvc.GetServiceName() {
				continue
			}
			for _, method := range svc.GetMethod() {
				fullServiceName := genFullServiceName(currentsvc.GetPackageName(), currentsvc.GetServiceName())
				if method.GetOptions() == nil {
					method.Options = &descriptor.MethodOptions{}
				}
				if err := proto.SetExtension(method.GetOptions(), annotations.E_Http, &annotations.HttpRule{
					Pattern: &annotations.HttpRule_Post{
						Post: httpPath(upstream, fullServiceName, method.GetName()),
					},
					Body: "*",
				}); err != nil {
					return errors.Wrap(err, "setting http extensions for method.Options")
				}
				log.Debugf("method.options: %v", *method.GetOptions())
			}
		}
	}

	return nil
}

func addWellKnownProtos(descriptors *descriptor.FileDescriptorSet) {
	var googleApiHttpFound, googleApiAnnotationsFound, googleApiDescriptorFound bool
	for _, file := range descriptors.GetFile() {
		log.Debugf("inspecting descriptor for proto file %v...", file.GetName())
		if file.GetName() == "google/api/http.proto" {
			googleApiHttpFound = true
			continue
		}
		if file.GetName() == "google/api/annotations.proto" {
			googleApiAnnotationsFound = true
			continue
		}
		if file.GetName() == "google/protobuf/descriptor.proto" {
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
		// TODO: investigate if we need this
		// addGoogleApisAnnotations(packageName, set)
	}
}

func httpPath(upstream *v1.Upstream, serviceName, methodName string) string {
	h := sha1.New()
	h.Write([]byte(upstream.GetMetadata().GetNamespace() + upstream.GetMetadata().GetName() + serviceName))
	return "/" + fmt.Sprintf("%x", h.Sum(nil))[:8] + "/" + upstream.GetMetadata().GetName() + "/" + serviceName + "/" + methodName
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
		for _, grpcsvc := range serviceAndDescriptor.Spec.GetGrpcServices() {
			fullName := genFullServiceName(grpcsvc.GetPackageName(), grpcsvc.GetServiceName())
			fullServiceNames = append(fullServiceNames, fullName)
		}
		filterConfig := &envoytranscoder.GrpcJsonTranscoder{
			DescriptorSet: &envoytranscoder.GrpcJsonTranscoder_ProtoDescriptorBin{
				ProtoDescriptorBin: descriptorBytes,
			},
			Services:                  fullServiceNames,
			MatchIncomingRequestRoute: true,
		}

		shf, err := plugins.NewStagedFilter(wellknown.GRPCJSONTranscoder, filterConfig, pluginStage)
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
