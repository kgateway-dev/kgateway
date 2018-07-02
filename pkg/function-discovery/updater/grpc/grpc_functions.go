package grpc

import (
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func GetFuncs(files dependencies.FileStorage, us *v1.Upstream) ([]*v1.Function, error) {
	if us.ServiceInfo == nil {
		return nil, errors.Errorf("no service info provided for %s", us.Name)
	}
	spec, err := grpc.DecodeServiceProperties(us.ServiceInfo.Properties)
	if err != nil {
		return nil, errors.Wrap(err, "decoding grpc service info")
	}
	f, err := files.Get(spec.DescriptorsFileRef)
	if err != nil {
		return nil, errors.Wrap(err, "getting descriptors file")
	}
	var descriptors descriptor.FileDescriptorSet
	if err := proto.Unmarshal(f.Contents, &descriptors); err != nil {
		return nil, errors.Wrap(err, "unmarshalling descriptors file")
	}
	var funcs []*v1.Function
	for _, file := range descriptors.File {
		for _, s := range file.Service {
			for _, m := range s.Method {
				funcs = append(funcs, &v1.Function{
					Name: *s.Name +"."+*m.Name,
				})
			}
		}
	}
	return funcs, nil
}

func IsGRPC(us *v1.Upstream) bool {
	return us.ServiceInfo != nil && us.ServiceInfo.Type == grpc.ServiceTypeGRPC
}