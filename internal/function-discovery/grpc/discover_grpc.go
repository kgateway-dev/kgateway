package grpc

import (
	"context"

	"strings"

	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/internal/function-discovery"
	"github.com/solo-io/gloo/internal/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	grpcplugin "github.com/solo-io/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"google.golang.org/grpc"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type grpcDetector struct {
	files dependencies.FileStorage
}

func NewGRPCDetector(files dependencies.FileStorage) detector.Interface {
	return &grpcDetector{
		files: files,
	}
}

// if it detects the upstream is a known functional type, give us the
// service info and annotations to mark it with
func (d *grpcDetector) DetectFunctionalService(us *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	log.Debugf("attempting to detect GRPC for %s", us.Name)
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "dialing grpc on %v", addr)
	}
	refClient := grpcreflect.NewClient(context.Background(), reflectpb.NewServerReflectionClient(cc))

	services, err := refClient.ListServices()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "listing services. are you sure %v implements reflection?", addr)
	}
	log.Printf("%v discovered as a gRPC service", addr)
	var (
		serviceNames []string
	)
	descriptors := &descriptor.FileDescriptorSet{}
	for _, s := range services {
		// ignore the reflection descriptor
		if s == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		files, err := getAllDescriptors(refClient, s)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting descriptors for svc %s", s)
		}
		descriptors.File = append(descriptors.File, files...)

		parts := strings.Split(s, ".")
		serviceSuffix := parts[len(parts)-1]
		serviceNames = append(serviceNames, serviceSuffix)
	}

	b, err := proto.Marshal(descriptors)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshalling proto descriptors")
	}

	// the : is a necessary separator for kube file storage
	// otherwise it's just ignored
	fileRef := fmt.Sprintf("%v:%v.descriptors",
		"grpc-discovery",
		strings.Join(serviceNames, "."))

	file := &dependencies.File{
		Ref:      fileRef,
		Contents: b,
	}

	if _, err := d.files.Create(file); err != nil {
		return nil, nil, errors.Wrap(err, "creating file for discovered descriptors")
	}

	svcInfo := &v1.ServiceInfo{
		Type: grpcplugin.ServiceTypeGRPC,
		Properties: grpcplugin.EncodeServiceProperties(grpcplugin.ServiceProperties{
			GRPCServiceNames:   serviceNames,
			DescriptorsFileRef: fileRef,
		}),
	}

	annotations := make(map[string]string)
	annotations[functiondiscovery.DiscoveryTypeAnnotationKey] = "grpc"
	return svcInfo, annotations, nil
}

func getAllDescriptors(refClient *grpcreflect.Client, s string) ([]*descriptor.FileDescriptorProto, error) {
	root, err := refClient.FileContainingSymbol(s)
	if err != nil {
		return nil, errors.Wrapf(err, "getting file for symbol %s", s)
	}
	files := getDepTree(root)
	return files, nil
}

func getDepTree(root *desc.FileDescriptor) []*descriptor.FileDescriptorProto {
	var deps []*descriptor.FileDescriptorProto
	for _, dep := range root.GetDependencies() {
		deps = append(deps, getDepTree(dep)...)
	}
	deps = append(deps, root.AsFileDescriptorProto())
	return deps
}
