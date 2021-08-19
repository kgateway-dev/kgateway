package grpc

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
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
	set.File = append([]*descriptor.FileDescriptorProto{&descriptorsDescriptor}, set.GetFile()...)
}

func addGoogleApisHttp(set *descriptor.FileDescriptorSet) {
	set.File = append([]*descriptor.FileDescriptorProto{&httpDescriptor}, set.GetFile()...)
}

func addGoogleApisAnnotations(packageName string, set *descriptor.FileDescriptorSet) {
	for _, file := range set.GetFile() {
		if file.GetPackage() == packageName {
			file.Dependency = append(file.GetDependency(), "google/api/annotations.proto")
		}
	}
	set.File = append([]*descriptor.FileDescriptorProto{&annotationsDescriptor}, set.GetFile()...)
}
