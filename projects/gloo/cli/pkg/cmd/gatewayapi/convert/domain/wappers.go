package domain

import (
	"fmt"

	gatewaykube "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	glookube "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	v1 "github.com/solo-io/solo-apis/pkg/api/enterprise.gloo.solo.io/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"
)

type Wrapper interface {
	GetName() string
	GetNamespace() string
	GetOriginalFileName() string
	NameIndex() string
	GetLabels() map[string]string
	GetObjectKind() schema.ObjectKind
}

type YAMLWrapper struct {
	runtime.Object
	OriginalFileName string
	Yaml             string
}

func (w *YAMLWrapper) GetLabels() map[string]string {
	return map[string]string{}
}
func (w *YAMLWrapper) GetName() string {
	return "unknown"
}
func (w *YAMLWrapper) GetNamespace() string {
	return "unknown"
}
func (w *YAMLWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}
func (w *YAMLWrapper) NameIndex() string {
	return "unknown-unknown"
}

type HTTPRouteWrapper struct {
	*gwv1.HTTPRoute
	OriginalFileName string
}

func (w *HTTPRouteWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}

func (w *HTTPRouteWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type RouteOptionWrapper struct {
	*gatewaykube.RouteOption
	OriginalFileName string
}

func (w *RouteOptionWrapper) NameIndex() string {
	return fmt.Sprintf("%s/%s", w.Namespace, w.Name)
}

func (w *RouteOptionWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type VirtualHostOptionWrapper struct {
	*gatewaykube.VirtualHostOption
	OriginalFileName string
}

func (w *VirtualHostOptionWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *VirtualHostOptionWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type ListenerOptionWrapper struct {
	*gatewaykube.ListenerOption
	OriginalFileName string
}

func (w *ListenerOptionWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *ListenerOptionWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type HTTPListenerOptionWrapper struct {
	*gatewaykube.HttpListenerOption
	OriginalFileName string
}

func (w *HTTPListenerOptionWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *HTTPListenerOptionWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type UpstreamWrapper struct {
	*glookube.Upstream
	OriginalFileName string
}

func (w *UpstreamWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *UpstreamWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type AuthConfigWrapper struct {
	*v1.AuthConfig
	OriginalFileName string
}

func (w *AuthConfigWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *AuthConfigWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type GatewayWrapper struct {
	*gwv1.Gateway
	OriginalFileName string
}

func (w *GatewayWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *GatewayWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type ListenerSetWrapper struct {
	*apixv1a1.XListenerSet
	OriginalFileName string
}

func (w *ListenerSetWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *ListenerSetWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type RouteTableWrapper struct {
	*gatewaykube.RouteTable
	OriginalFileName string
}

func (w *RouteTableWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *RouteTableWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type VirtualServiceWrapper struct {
	*gatewaykube.VirtualService
	OriginalFileName string
}

func (w *VirtualServiceWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *VirtualServiceWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type GlooGatewayWrapper struct {
	*gatewaykube.Gateway
	OriginalFileName string
}

func (w *GlooGatewayWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *GlooGatewayWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}

type DirectResponseWrapper struct {
	*v1alpha1.DirectResponse
	OriginalFileName string
}

func (w *DirectResponseWrapper) NameIndex() string {
	return fmt.Sprintf("%s-%s", w.Namespace, w.Name)
}
func (w *DirectResponseWrapper) GetOriginalFileName() string {
	return w.OriginalFileName
}
