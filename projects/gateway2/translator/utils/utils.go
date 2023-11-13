package utils

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	proto_old "github.com/golang/protobuf/proto"
	corev3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func ToAny(pb proto.Message) *anypb.Any {
	any := &anypb.Any{}
	err := anypb.MarshalFrom(any, pb, proto.MarshalOptions{Deterministic: true})
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
	return SanitizeXdsName(fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort))
}

func SanitizeXdsName(name string) string {
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
	name = strings.Replace(name, ".", "-", -1)
	name = strings.ToLower(name)
	return name
}

// Produce the same hash regardless of order, so we don't need to sort lists.
func EnvoyCacheResourcesListSetToFnvHash(resources []envoycache.Resource) (uint64, error) {
	var finalHash uint64
	// 8kb capacity, consider raising if we find the buffer is frequently being
	// re-allocated by MarshalAppend to fit larger protos.
	// the goal is to keep allocations constant for GC, without allocating an
	// unnecessarily large buffer.
	buffer := make([]byte, 0, 8*1024)
	for _, r := range resources {
		// proto.MessageV2 will create another allocation, updating solo-kit
		// to use google protos (rather than github protos, i.e. use v2) is
		// another path to further improve performance here.
		hash, err := ProtoFnvHash(buffer, proto_old.MessageV2(r.ResourceProto()))
		if err != nil {
			return 0, err
		}
		finalHash ^= hash
	}
	return finalHash, nil
}

func ProtoFnvHash(buffer []byte, r proto.Message) (uint64, error) {
	hasher := fnv.New64()
	mo := proto.MarshalOptions{Deterministic: true}
	buf := buffer[:0]
	out, err := mo.MarshalAppend(buf, r)
	if err != nil {
		err := fmt.Errorf("marshalling envoy snapshot components: %w", err)
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return 0, err
	}
	_, err = hasher.Write(out)
	if err != nil {
		err := fmt.Errorf("constructing hash for envoy snapshot components: %w", err)
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return 0, err
	}
	return hasher.Sum64(), nil
}
