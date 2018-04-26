package main

import (
	"flag"
	"io/ioutil"

	"github.com/gogo/googleapis/google/api"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/solo-io/gloo/pkg/log"
)

func main() {
	descriptorsFile := flag.String("f", "descriptors.proto", "descriptors filename")
	flag.Parse()
	run(*descriptorsFile)
}

func run(f string) {
	b, err := ioutil.ReadFile(f)
	must(err)
	set, err := convertProto(b)
	must(err)
	log.Printf("%v", set)
	logProtoMethodHttpRules(set)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func convertProto(b []byte) (*descriptor.FileDescriptorSet, error) {
	var fileDescriptor descriptor.FileDescriptorSet
	err := proto.Unmarshal(b, &fileDescriptor)
	return &fileDescriptor, err
}

func logProtoMethodHttpRules(set *descriptor.FileDescriptorSet) {
	for _, file := range set.File {
		log.Printf("file message type: %v", file.MessageType)
		for _, svc := range file.Service {
			log.Printf("service name: %v", svc.Name)
			for _, method := range svc.Method {
				log.Printf("method name: %v", method.Name)
				g, err := proto.GetExtension(method.Options, api.E_Http)
				if err != nil {
					log.Printf("missing http option on the extensions, skipping: %v", *method.Name)
					continue
				}
				httpRule, ok := g.(*api.HttpRule)
				if !ok {
					panic(g)
				}
				log.Printf("rule: %v", httpRule)
			}
		}
	}
}
