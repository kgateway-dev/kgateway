package grpc

import (
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
)

//go:generate sh -c "2goarray annotationsDescriptorBytes grpc < google/api/annotations.proto.descriptor  | sed 's@// date.*@@g' > annotations.google.descriptor.go"
//go:generate sh -c "2goarray httpDescriptorBytes grpc < google/api/http.proto.descriptor | sed 's@// date.*@@g' > http.google.descriptor.go"
//go:generate sh -c "2goarray descriptorsDescriptorBytes grpc < google/api/descriptors.proto.descriptor | sed 's@// date.*@@g' > descriptors.google.descriptor.go"

var annotationsDescriptor, httpDescriptor, descriptorsDescriptor descriptor.FileDescriptorProto

func init() {
	err := proto.Unmarshal(annotationsDescriptorBytes, &annotationsDescriptor)
	if err != nil {
		panic(err)
	}
	err = proto.Unmarshal(httpDescriptorBytes, &httpDescriptor)
	if err != nil {
		panic(err)
	}
	err = proto.Unmarshal(descriptorsDescriptorBytes, &descriptorsDescriptor)
	if err != nil {
		panic(err)
	}
}

func addGoogleApisDescriptor(set *descriptor.FileDescriptorSet) {
	set.File = append([]*descriptor.FileDescriptorProto{&descriptorsDescriptor}, set.File...)
}

func addGoogleApisHttp(set *descriptor.FileDescriptorSet) {
	set.File = append([]*descriptor.FileDescriptorProto{&httpDescriptor}, set.File...)
}

func addGoogleApisAnnotations(packageName string, set *descriptor.FileDescriptorSet) {
	for _, file := range set.File {
		if *file.Package == packageName {
			file.Dependency = append(file.Dependency, "google/api/annotations.proto")
		}
	}
	set.File = append([]*descriptor.FileDescriptorProto{&annotationsDescriptor}, set.File...)
}
