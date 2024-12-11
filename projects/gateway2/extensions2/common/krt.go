package common

import (
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
)

type CommonCollections struct {
	Client   kube.Client
	KrtDbg   *krt.DebugHandler
	Secrets  *krtcollections.SecretIndex
	Pods     krt.Collection[krtcollections.LocalityPod]
	Settings krt.Singleton[glookubev1.Settings]
}
