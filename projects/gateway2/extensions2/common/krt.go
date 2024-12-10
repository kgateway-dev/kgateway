package common

import (
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
)

type CommonCollections struct {
	Client  kube.Client
	KrtDbg  *krt.DebugHandler
	Secrets *krtcollections.SecretIndex
}
