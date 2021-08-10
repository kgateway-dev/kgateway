package grpc

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	grpc_plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc"
	"github.com/solo-io/go-utils/contextutils"
	"google.golang.org/grpc"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func getgrpcspec(u *v1.Upstream) *grpc_plugins.ServiceSpec {
	upstreamType, ok := u.GetUpstreamType().(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}

	if upstreamType.GetServiceSpec() == nil {
		return nil
	}

	grpcwrapper, ok := upstreamType.GetServiceSpec().GetPluginType().(*plugins.ServiceSpec_Grpc)
	if !ok {
		return nil
	}
	grpc := grpcwrapper.Grpc
	return grpc
}

// ilackarms: this is the root object
type FunctionDiscoveryFactory struct {
	// TODO: yuval-k: integrate backoff stuff here
	DetectionTimeout   time.Duration
	DetectionRetryBase time.Duration
	FunctionPollTime   time.Duration
	// TODO: move over to ArtifactClient
	Artifacts v1.ArtifactClient
}

func (f *FunctionDiscoveryFactory) NewFunctionDiscovery(u *v1.Upstream) fds.UpstreamFunctionDiscovery {
	return &UpstreamFunctionDiscovery{
		upstream: u,
	}
}

type UpstreamFunctionDiscovery struct {
	upstream *v1.Upstream
}

func (f *UpstreamFunctionDiscovery) IsFunctional() bool {
	return getgrpcspec(f.upstream) != nil
}

func (f *UpstreamFunctionDiscovery) DetectType(ctx context.Context, url *url.URL) (*plugins.ServiceSpec, error) {
	log := contextutils.LoggerFrom(ctx)
	log.Debugf("attempting to detect GRPC for %s", f.upstream.GetMetadata().GetName())

	refClient, closeConn, err := getclient(ctx, url)
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
	for {
		// TODO: get backoff values from config?
		err := contextutils.NewExponentioalBackoff(contextutils.ExponentioalBackoff{}).Backoff(ctx, func(ctx context.Context) error {
			return f.DetectFunctionsOnce(ctx, url, updatecb)
		})

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// ignore other errors as we would like to continue forever.
		}

		// sleep so we are not hogging
		// TODO(yuval-k): customize time to sleep in config
		if err := contextutils.Sleep(ctx, time.Minute); err != nil {
			return err
		}
	}
}

func (f *UpstreamFunctionDiscovery) DetectFunctionsOnce(ctx context.Context, url *url.URL, updatecb func(fds.UpstreamMutator) error) error {
	log := contextutils.LoggerFrom(ctx)

	log.Infof("%v discovered as a gRPC service", url)

	refClient, closeConn, err := getclient(ctx, url)
	if err != nil {
		return err
	}
	defer closeConn()

	services, err := refClient.ListServices()
	if err != nil {
		return errors.Wrapf(err, "listing services. are you sure %v implements reflection?", url)
	}

	descriptors := &descriptor.FileDescriptorSet{}

	var grpcservices []*grpc_plugins.ServiceSpec_GrpcService

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
		grpcservice := &grpc_plugins.ServiceSpec_GrpcService{
			PackageName: servicePackage,
			ServiceName: serviceName,
		}
		// find the service in the file and get its functions
		for _, svc := range root.GetServices() {
			if svc.GetName() == serviceName {
				methods := svc.GetMethods()
				for _, method := range methods {
					methodname := method.GetName()
					grpcservice.FunctionNames = append(grpcservice.GetFunctionNames(), methodname)
				}
			}
		}
		grpcservices = append(grpcservices, grpcservice)
	}

	rawDescriptors, err := proto.Marshal(descriptors)
	if err != nil {
		return errors.Wrap(err, "marshalling proto descriptors")
	}

	encodedDescriptors := []byte(base64.StdEncoding.EncodeToString(rawDescriptors))

	return updatecb(func(out *v1.Upstream) error {
		svcspec := getgrpcspec(out)
		if svcspec == nil {
			return errors.New("not a GRPC upstream")
		}
		// TODO(yuval-k): ideally GrpcServices should be google.protobuf.FileDescriptorSet
		//  but that doesn't work with gogoproto.equal_all.
		svcspec.GrpcServices = grpcservices
		svcspec.Descriptors = encodedDescriptors
		return nil
	})
}

func getclient(ctx context.Context, url *url.URL) (*grpcreflect.Client, func() error, error) {
	var dialopts []grpc.DialOption
	if url.Scheme != "https" {
		dialopts = append(dialopts, grpc.WithInsecure())
	}

	cc, err := grpc.Dial(url.Host, dialopts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "dialing grpc on %v", url.Host)
	}
	refClient := grpcreflect.NewClient(ctx, reflectpb.NewServerReflectionClient(cc))
	return refClient, cc.Close, nil
}

func getDepTree(root *desc.FileDescriptor) []*descriptor.FileDescriptorProto {
	var deps []*descriptor.FileDescriptorProto
	for _, dep := range root.GetDependencies() {
		deps = append(deps, getDepTree(dep)...)
	}
	deps = append(deps, root.AsFileDescriptorProto())
	return deps
}
