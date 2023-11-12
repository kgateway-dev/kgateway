package utils

import (
	"crypto/md5"
	"fmt"
	"strings"

	corev3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func ToAny(pb proto.Message) *anypb.Any {
	any, err := anypb.New(pb)
	if err != nil {
		// all config types should already be known
		// therefore this should never happen
		panic(err)
	}
	return any
}

func NewTyped(name string, pb proto.Message) *corev3.TypedExtensionConfig {
	return &corev3.TypedExtensionConfig{
		Name:        name,
		TypedConfig: ToAny(pb),
	}
}

func ClusterName(serviceNamespace, serviceName string, servicePort int32) string {
	return SanitizeNameV2(fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort))
}

func SanitizeNameV2(name string) string {
	name = strings.Replace(name, "*", "-", -1)
	name = strings.Replace(name, "/", "-", -1)
	name = strings.Replace(name, ".", "-", -1)
	name = strings.Replace(name, "[", "", -1)
	name = strings.Replace(name, "]", "", -1)
	name = strings.Replace(name, ":", "-", -1)
	name = strings.Replace(name, "_", "-", -1)
	name = strings.Replace(name, " ", "-", -1)
	name = strings.Replace(name, "\n", "", -1)
	name = strings.Replace(name, "\"", "", -1)
	name = strings.Replace(name, "'", "", -1)
	if len(name) > 63 {
		hash := md5.Sum([]byte(name))
		name = fmt.Sprintf("%s-%x", name[:31], hash)
		name = name[:63]
	}
	name = strings.Replace(name, ".", "-", -1)
	name = strings.ToLower(name)
	return name
}
