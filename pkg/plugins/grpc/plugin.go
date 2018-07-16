package grpc

import (
	"crypto/sha1"
	"fmt"
	"strings"

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

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types:${GOPATH}/src service_properties.proto

type ServicesAndDescriptor struct {
	ServiceNames []string
	PackageNames []string
	Descriptors  *descriptor.FileDescriptorSet
}

func NewPlugin() *Plugin {
	return &Plugin{
		upstreamServices: make(map[string]ServicesAndDescriptor),
		transformation:   transformation.NewTransformationPlugin(),
	}
}

type Plugin struct {
	// keep track of which descriptor belongs to which upstream
	upstreamServices map[string]ServicesAndDescriptor
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
	serviceNames := serviceProperties.GrpcServiceNames

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

	var packageNames []string

	for _, serviceName := range serviceNames {
		packageName, err := addHttpRulesToProto(in.Name, serviceName, descriptors)
		if err != nil {
			return errors.Wrapf(err, "failed to generate http rules for service %s in proto descriptors", serviceName)
		}
		packageNames = append(packageNames, packageName)
	}

	// cache the descriptors; we'll need then when we create our grpc filters
	// need the package name as well, required by the transcoder filter
	// keep track of which descriptor and services belongs to which upstream
	// note: the descriptor is the same
	p.upstreamServices[in.Name] = ServicesAndDescriptor{
		Descriptors:  descriptors,
		PackageNames: packageNames,
		ServiceNames: serviceNames,
	}

	addWellKnownProtos(descriptors)

	out.Http2ProtocolOptions = &envoycore.Http2ProtocolOptions{}

	p.transformation.ActivateFilterForCluster(out)

	return nil
}

func genFullServiceName(packageName, serviceName string) string {
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
					Path: &types.StringValue{Value: path},
				},
			})
		}
	}
	return p.transformation.AddRequestTransformationsToRoute(p.templateForFunction, in, out)
}

func (p *Plugin) templateForFunction(dest *v1.Destination_Function) (*transformation.TransformationTemplate, error) {
	upstreamName := dest.Function.UpstreamName
	servicesAndDescriptor, ok := p.upstreamServices[upstreamName]
	if !ok {
		// the upstream is not a grpc desintation
		return nil, nil
	}

	// get the package_name.service_name to generate the path that envoy wants
	var (
		fullServiceName string
		methodName      string
	)

	// for multi-service upstreams: format should be ServiceName.MethodName
	// for single service upstreams, format can be MethodName without service name
	serviceAndMethodName := dest.Function.FunctionName
	split := strings.SplitN(serviceAndMethodName, ".", 2)
	switch {
	case len(split) == 2:
		methodName = split[1]
		for i, svcName := range servicesAndDescriptor.ServiceNames {
			if svcName == split[0] {
				fullServiceName = genFullServiceName(servicesAndDescriptor.PackageNames[i], svcName)
				break
			}
		}
		if fullServiceName == "" {
			return nil, errors.Errorf("service %v was not found for grpc upstream %s", split[0], upstreamName)
		}
	default:
		// if service name isn't included && the upstream only provides one service, we can use just the method name
		methodName = serviceAndMethodName
		if len(servicesAndDescriptor.ServiceNames) != 1 {
			return nil, errors.Errorf("grpc upstream %v contains %v services. the route's function_name destination must follow"+
				" the format 'ServiceName.MethodName'. available services for this upstream: %v", servicesAndDescriptor.ServiceNames)
		}
		fullServiceName = genFullServiceName(servicesAndDescriptor.PackageNames[0], servicesAndDescriptor.ServiceNames[0])
	}

	// create the transformation for the route
	outPath := httpPath(upstreamName, fullServiceName, methodName)

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
					fullServiceName := genFullServiceName(packageName, serviceName)
					if method.Options == nil {
						method.Options = &descriptor.MethodOptions{}
					}
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

func (p *Plugin) HttpFilters(_ *plugins.HttpFilterPluginParams) []plugins.StagedHttpFilter {
	defer func() {
		// clear cache
		p.upstreamServices = make(map[string]ServicesAndDescriptor)
	}()

	if len(p.upstreamServices) == 0 {
		return nil
	}

	transformationFilter := p.transformation.GetTransformationFilter()
	if transformationFilter == nil {
		log.Warnf("ERROR: nil transformation filter returned from transformation plugin")
		return nil
	}

	var filters []plugins.StagedHttpFilter
	for _, serviceAndDescriptor := range p.upstreamServices {
		descriptorBytes, err := proto.Marshal(serviceAndDescriptor.Descriptors)
		if err != nil {
			log.Warnf("ERROR: marshaling proto descriptor: %v", err)
			continue
		}
		var fullServiceNames []string
		for i := range serviceAndDescriptor.ServiceNames {
			fullName := genFullServiceName(serviceAndDescriptor.PackageNames[i], serviceAndDescriptor.ServiceNames[i])
			fullServiceNames = append(fullServiceNames, fullName)
		}
		filterConfig, err := util.MessageToStruct(&envoytranscoder.GrpcJsonTranscoder{
			DescriptorSet: &envoytranscoder.GrpcJsonTranscoder_ProtoDescriptorBin{
				ProtoDescriptorBin: descriptorBytes,
			},
			Services:                  fullServiceNames,
			MatchIncomingRequestRoute: true,
		})
		if err != nil {
			log.Warnf("ERROR: marshaling GrpcJsonTranscoder config: %v", err)
			return nil
		}
		filters = append(filters, plugins.StagedHttpFilter{
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
	filters = append([]plugins.StagedHttpFilter{*transformationFilter}, filters...)

	return filters
}

// just so the init plugin knows we're functional
func (p *Plugin) ParseFunctionSpec(params *plugins.FunctionPluginParams, in v1.FunctionSpec) (*types.Struct, error) {
	return nil, nil
}
