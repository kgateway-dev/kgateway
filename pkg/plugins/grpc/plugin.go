package grpc

import (
	"crypto/sha1"
	"fmt"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoytranscoder "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/transcoder/v2"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/googleapis/google/api"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"

	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/plugins/common/transformation"
)

func init() {
	plugins.Register(NewPlugin())
}

type ServiceAndDescriptors struct {
	FullServiceName string
	Descriptors     *descriptor.FileDescriptorSet
}

func NewPlugin() *Plugin {
	return &Plugin{
		upstreamServices: make(map[string]ServiceAndDescriptors),
		transformation:   transformation.NewTransformationPlugin(),
	}
}

type Plugin struct {
	// map service names to their descriptors
	// keep track of which service belongs to which upstream
	upstreamServices map[string]ServiceAndDescriptors
	transformation   transformation.Plugin
}

const (
	filterName  = "envoy.grpc_json_transcoder"
	pluginStage = plugins.PreOutAuth

	ServiceTypeGRPC = "gRPC"
)

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {
	deps := &plugins.Dependencies{}
	for _, us := range cfg.Upstreams {
		if !isOurs(us) {
			continue
		}
		serviceSpec, err := DecodeServiceProperties(us.ServiceInfo.Properties)
		if err != nil {
			log.Warnf("%v: error parsing service properties for upstream %v: %v",
				ServiceTypeGRPC, us.Name, err)
			continue
		}
		deps.FileRefs = append(deps.FileRefs, serviceSpec.DescriptorsFileRef)
	}
	return deps
}

func isOurs(in *v1.Upstream) bool {
	return in.ServiceInfo != nil && in.ServiceInfo.Type == ServiceTypeGRPC
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if !isOurs(in) {
		return nil
	}

	serviceProperties, err := DecodeServiceProperties(in.ServiceInfo.Properties)
	if err != nil {
		return errors.Wrap(err, "parsing service properties")
	}
	fileRef := serviceProperties.DescriptorsFileRef
	serviceNames := serviceProperties.GRPCServiceNames

	if fileRef == "" {
		return errors.New("service_info.properties.descriptors_file_ref cannot be empty")
	}
	if len(serviceNames) == 0 {
		return errors.New("service_info.properties.service_names cannot be empty")
	}
	descriptorsFile, ok := params.Files[fileRef]
	if !ok {
		return errors.Errorf("descriptors file not found for file ref %v", fileRef)
	}
	descriptors, err := convertProto(descriptorsFile.Contents)
	if err != nil {
		return errors.Wrapf(err, "parsing file %v as a proto descriptor set", fileRef)
	}

	for _, serviceName := range serviceNames {
		packageName, err := addHttpRulesToProto(in.Name, serviceName, descriptors)
		if err != nil {
			return errors.Wrapf(err, "failed to generate http rules for service %s in proto descriptors", serviceName)
		}
		// cache the descriptors; we'll need then when we create our grpc filters
		// need the package name as well, required by the transcoder filter
		fullServiceName := genFullServiceName(in.Name, packageName, serviceName)
		// keep track of which service belongs to which upstream
		p.upstreamServices[in.Name] = ServiceAndDescriptors{
			Descriptors: descriptors, FullServiceName: fullServiceName}
	}

	addWellKnownProtos(descriptors)

	out.Http2ProtocolOptions = &envoycore.Http2ProtocolOptions{}

	p.transformation.ActivateFilterForCluster(out)

	return nil
}

func genFullServiceName(upstreamName, packageName, serviceName string) string {
	return packageName + "." + serviceName
}

func convertProto(b []byte) (*descriptor.FileDescriptorSet, error) {
	var fileDescriptor descriptor.FileDescriptorSet
	err := proto.Unmarshal(b, &fileDescriptor)
	return &fileDescriptor, err
}

func getPath(matcher *v1.RequestMatcher) string {
	switch path := matcher.Path.(type) {
	case *v1.RequestMatcher_PathPrefix:
		return path.PathPrefix
	case *v1.RequestMatcher_PathExact:
		return path.PathExact
	case *v1.RequestMatcher_PathRegex:
		return path.PathRegex
	}
	panic("invalid matcher")
}

func (p *Plugin) ProcessRoute(_ *plugins.RoutePluginParams, in *v1.Route, out *envoyroute.Route) error {
	if in.Extensions == nil {
		matcher, ok := in.Matcher.(*v1.Route_RequestMatcher)
		if ok {
			path := getPath(matcher.RequestMatcher) + "?{query_string}"
			in.Extensions = transformation.EncodeRouteExtension(transformation.RouteExtension{
				Parameters: &transformation.Parameters{
					Path: &path,
				},
			})
		}
	}
	return p.transformation.AddRequestTransformationsToRoute(p.templateForFunction, in, out)
}

func (p *Plugin) templateForFunction(dest *v1.Destination_Function) (*transformation.TransformationTemplate, error) {
	upstreamName := dest.Function.UpstreamName
	serviceAndDescriptor, ok := p.upstreamServices[upstreamName]
	if !ok {
		// the upstream is not a grpc desintation
		return nil, nil
	}

	// method name should be function name in this case. TODO: document in the api
	methodName := dest.Function.FunctionName

	// create the transformation for the route

	outPath := httpPath(upstreamName, serviceAndDescriptor.FullServiceName, methodName)

	// add query matcher to out path. kombina for now
	// TODO: support query for matching
	outPath += `?{{ default(query_string), "")}}`

	// we always choose post
	httpMethod := "POST"
	return &transformation.TransformationTemplate{
		Headers: map[string]*transformation.InjaTemplate{
			":method": {Text: httpMethod},
			":path":   {Text: outPath},
		},
		BodyTransformation: &transformation.TransformationTemplate_MergeExtractorsToBody{
			MergeExtractorsToBody: &transformation.MergeExtractorsToBody{},
		},
	}, nil
}

// returns package name
func addHttpRulesToProto(upstreamName, serviceName string, set *descriptor.FileDescriptorSet) (string, error) {
	var packageName string
	for _, file := range set.File {
	findService:
		for _, svc := range file.Service {
			if *svc.Name == serviceName {
				for _, method := range svc.Method {
					packageName = *file.Package
					fullServiceName := genFullServiceName(upstreamName, packageName, serviceName)
					if err := proto.SetExtension(method.Options, api.E_Http, &api.HttpRule{
						Pattern: &api.HttpRule_Post{
							Post: httpPath(upstreamName, fullServiceName, *method.Name),
						},
						Body: "*",
					}); err != nil {
						return "", errors.Wrap(err, "setting http extensions for method.Options")
					}
					log.Debugf("method.options: %v", *method.Options)
				}
				break findService
			}
		}
	}

	if packageName == "" {
		return "", errors.Errorf("could not find match: %v/%v", upstreamName, serviceName)
	}
	return packageName, nil
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

func httpPath(upstreamName, serviceName, methodName string) string {
	h := sha1.New()
	h.Write([]byte(upstreamName + serviceName))
	return "/" + fmt.Sprintf("%x", h.Sum(nil))[:8] + "/" + upstreamName + "/" + serviceName + "/" + methodName
}

func (p *Plugin) HttpFilters(_ *plugins.FilterPluginParams) []plugins.StagedFilter {
	defer func() {
		// clear cache
		p.upstreamServices = make(map[string]ServiceAndDescriptors)
	}()

	if len(p.upstreamServices) == 0 {
		return nil
	}

	transformationFilter := p.transformation.GetTransformationFilter()
	if transformationFilter == nil {
		log.Warnf("ERROR: nil transformation filter returned from transformation plugin")
		return nil
	}

	var filters []plugins.StagedFilter
	for _, serviceAndDescriptor := range p.upstreamServices {
		descriptorBytes, err := proto.Marshal(serviceAndDescriptor.Descriptors)
		if err != nil {
			log.Warnf("ERROR: marshaling proto descriptor: %v", err)
			continue
		}
		//log.Debugf("service %v using descriptors %v", serviceName, protoDescriptor.File)
		filterConfig, err := util.MessageToStruct(&envoytranscoder.GrpcJsonTranscoder{
			DescriptorSet: &envoytranscoder.GrpcJsonTranscoder_ProtoDescriptorBin{
				ProtoDescriptorBin: descriptorBytes,
			},
			Services:                  []string{serviceAndDescriptor.FullServiceName},
			MatchIncomingRequestRoute: true,
		})
		if err != nil {
			log.Warnf("ERROR: marshaling GrpcJsonTranscoder config: %v", err)
			return nil
		}
		filters = append(filters, plugins.StagedFilter{
			HttpFilter: &envoyhttp.HttpFilter{
				Name:   filterName,
				Config: filterConfig,
			},
			Stage: pluginStage,
		})
	}

	if len(filters) == 0 {
		log.Warnf("ERROR: no valid GrpcJsonTranscoder available")
		return nil
	}
	filters = append([]plugins.StagedFilter{*transformationFilter}, filters...)

	return filters
}

// just so the init plugin knows we're functional
func (p *Plugin) ParseFunctionSpec(params *plugins.FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error) {
	if params.ServiceType != ServiceTypeGRPC {
		return nil, nil
	}
	return nil, errors.New("functions are not required for service type " + ServiceTypeGRPC)
}
