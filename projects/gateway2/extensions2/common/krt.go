package common

import (
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/pkg/client/clientset/versioned"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
)

type CommonCollections struct {
	OurClient versioned.Interface
	Client    kube.Client
	KrtOpts   krtutil.KrtOptions
	Secrets   *krtcollections.SecretIndex
	Pods      krt.Collection[krtcollections.LocalityPod]
	Settings  krt.Singleton[glookubev1.Settings]
	RefGrants *krtcollections.RefGrantIndex
}
