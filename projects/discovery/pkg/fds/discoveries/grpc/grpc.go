package grpc

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/grpc"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	grpc_plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
)

func getGrpcspec(u *v1.Upstream) *grpc_plugins.ServiceSpec {
	upstreamType, ok := u.GetUpstreamType().(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}

	if upstreamType.GetServiceSpec() == nil {
		return nil
	}

	grpcWrapper, ok := upstreamType.GetServiceSpec().GetPluginType().(*plugins.ServiceSpec_Grpc)
	if !ok {
		return nil
	}
	return grpcWrapper.Grpc
}

func NewFunctionDiscoveryFactory() fds.FunctionDiscoveryFactory {
	return &FunctionDiscoveryFactory{
		DetectionTimeout: time.Minute,
		FunctionPollTime: time.Second * 15,
	}
}

// FunctionDiscoveryFactory returns a FunctionDiscovery that can be used to discover functions
// ilackarms: this is the root object
type FunctionDiscoveryFactory struct {
	// TODO: yuval-k: integrate backoff stuff here
	DetectionTimeout   time.Duration
	DetectionRetryBase time.Duration
	FunctionPollTime   time.Duration
	// TODO: move over to ArtifactClient
	Artifacts v1.ArtifactClient
}

// NewFunctionDiscovery returns a FunctionDiscovery that can be used to discover functions
func (f *FunctionDiscoveryFactory) NewFunctionDiscovery(u *v1.Upstream, _ fds.AdditionalClients) fds.UpstreamFunctionDiscovery {
	return &UpstreamFunctionDiscovery{
		upstream: u,
	}
}

// UpstreamFunctionDiscovery represents a function discovery for upstream
type UpstreamFunctionDiscovery struct {
	upstream *v1.Upstream
}

// IsFunctional returns true if the upstream is functional
func (f *UpstreamFunctionDiscovery) IsFunctional() bool {
	return getGrpcspec(f.upstream) != nil
}

func (f *UpstreamFunctionDiscovery) DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error) {
	log := contextutils.LoggerFrom(ctx)
	log.Debugf("attempting to detect GRPC for %s", f.upstream.GetMetadata().GetName())

	refClient, closeConn, err := getClient(ctx, url)
	if err != nil {
		return nil, err
	}

	defer closeConn()

	_, err = refClient.ListServices()
	if err != nil {
		return nil, errors.Wrapf(err, "listing services. are you sure %v implements reflection?", url)
	}

	svcInfo := &plugins.ServiceSpec{
		PluginType: &plugins.ServiceSpec_Grpc{
			Grpc: &grpc_plugins.ServiceSpec{},
		},
	}
	return svcInfo, nil
}

func (f *UpstreamFunctionDiscovery) DetectFunctions(ctx context.Context, url *url.URL, _ func() fds.Dependencies, updatecb func(fds.UpstreamMutator) error) error {
	// TODO: get backoff values from config?
	err := contextutils.NewExponentioalBackoff(contextutils.ExponentioalBackoff{}).Backoff(ctx, func(ctx context.Context) error {
		return f.DetectFunctionsOnce(ctx, url, updatecb)
	})
	if err != nil {
		if ctx.Err() != nil {
			return multierror.Append(err, ctx.Err())
		}
		// only log other errors as we would like to continue forever.
		contextutils.LoggerFrom(ctx).Warnf("Unable to perform grpc function discovery for upstream %s in namespace %s, error: ",
			f.upstream.GetMetadata().GetName(),
			f.upstream.GetMetadata().GetNamespace(),
			err.Error(),
		)
	}

	// sleep so we are not hogging
	// TODO(yuval-k): customize time to sleep in config
	if err := contextutils.Sleep(ctx, time.Minute); err != nil {
		return err
	}
	return nil
}

func (f *UpstreamFunctionDiscovery) DetectFunctionsOnce(ctx context.Context, url *url.URL, updatecb func(fds.UpstreamMutator) error) error {
	log := contextutils.LoggerFrom(ctx)

	log.Infof("%v discovered as a gRPC service", url)

	refClient, closeConn, err := getClient(ctx, url)
	if err != nil {
		return err
	}
	defer closeConn()

	services, err := refClient.ListServices()
	if err != nil {
		return errors.Wrapf(err, "listing services. are you sure %v implements reflection?", url)
	}

	descriptors := &descriptor.FileDescriptorSet{}

	var grpcServices []*grpc_plugins.ServiceSpec_GrpcService

	for _, s := range services {
		// ignore the reflection descriptor
		if s == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		// TODO(yuval-k): do not add the same file twice
		root, err := refClient.FileContainingSymbol(s)
		if err != nil {
			return errors.Wrapf(err, "getting file for svc symbol %s", s)
		}
		files := getDepTree(root)

		descriptors.File = append(descriptors.GetFile(), files...)

		parts := strings.Split(s, ".")
		serviceName := parts[len(parts)-1]
		servicePackage := strings.Join(parts[:len(parts)-1], ".")
		grpcService := &grpc_plugins.ServiceSpec_GrpcService{
			PackageName: servicePackage,
			ServiceName: serviceName,
		}
		// find the service in the file and get its functions
		for _, svc := range root.GetServices() {
			if svc.GetName() == serviceName {
				methods := svc.GetMethods()
				for _, method := range methods {
					methodName := method.GetName()
					grpcService.FunctionNames = append(grpcService.GetFunctionNames(), methodName)
				}
			}
		}
		grpcServices = append(grpcServices, grpcService)
	}

	rawDescriptors, err := proto.Marshal(descriptors)
	if err != nil {
		return errors.Wrap(err, "marshalling proto descriptors")
	}

	encodedDescriptors := []byte(base64.StdEncoding.EncodeToString(rawDescriptors))

	return updatecb(func(out *v1.Upstream) error {
		svcSpec := getGrpcspec(out)
		if svcSpec == nil {
			return errors.New("not a GRPC upstream")
		}
		// TODO(yuval-k): ideally GrpcServices should be google.protobuf.FileDescriptorSet
		//  but that doesn't work with gogoproto.equal_all.
		svcSpec.GrpcServices = grpcServices
		svcSpec.Descriptors = encodedDescriptors
		return nil
	})
}

func getClient(ctx context.Context, url *url.URL) (*grpcreflect.Client, func() error, error) {
	var dialOpts []grpc.DialOption
	if url.Scheme != "https" {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	cc, err := grpc.Dial(url.Host, dialOpts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "dialing grpc on %v", url.Host)
	}
	refClient := grpcreflect.NewClient(ctx, reflectpb.NewServerReflectionClient(cc))
	closeConn := func() error {
		refClient.Reset()
		return cc.Close()
	}
	return refClient, closeConn, nil
}

func getDepTree(root *desc.FileDescriptor) []*descriptor.FileDescriptorProto {
	var deps []*descriptor.FileDescriptorProto
	for _, dep := range root.GetDependencies() {
		deps = append(deps, getDepTree(dep)...)
	}
	deps = append(deps, root.AsFileDescriptorProto())
	return deps
}
