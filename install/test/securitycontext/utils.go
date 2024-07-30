package securitycontext

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

const (
	defaultRunAsUser = 10101 // from values-template.yaml
)

// ApplyContainerSecurityDefaults describes a function that modifies a SecurityContext
// These functions are used in testing to modify the default "expected" security context of a container to match the template-specific defaults
type ApplyContainerSecurityDefaults func(*corev1.SecurityContext)

// ApplyNilSecurityDefaults is a function that does nothing and can be used as a default value for ApplyContainerSecurityDefaults
var ApplyNilSecurityDefaults = ApplyContainerSecurityDefaults(func(securityContext *corev1.SecurityContext) {})

var ApplyDiscoverySecurityDefaults = ApplyContainerSecurityDefaults(func(securityContext *corev1.SecurityContext) {
	securityContext.ReadOnlyRootFilesystem = ptr.To(true)
	securityContext.RunAsUser = ptr.To(int64(defaultRunAsUser))
})

// ApplyRunAsUserSecurityDefaults will update the runAsUser fields of the security context to the default value
var ApplyRunAsUserSecurityDefaults = ApplyContainerSecurityDefaults(func(securityContext *corev1.SecurityContext) {
	securityContext.RunAsUser = ptr.To(int64(defaultRunAsUser))
})

// ApplyKnativeSecurityDefaults updates the security context to match the defaults for Knative services
var ApplyKnativeSecurityDefaults = ApplyContainerSecurityDefaults(func(securityContext *corev1.SecurityContext) {
	securityContext.RunAsUser = ptr.To(int64(defaultRunAsUser))
	securityContext.ReadOnlyRootFilesystem = ptr.To(true)
	securityContext.Capabilities = &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
		Add:  []corev1.Capability{"NET_BIND_SERVICE"},
	}
})

var ApplyClusterIngressSecurityDefaults = ApplyContainerSecurityDefaults(func(securityContext *corev1.SecurityContext) {
	securityContext.Capabilities = &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
		Add:  []corev1.Capability{"NET_BIND_SERVICE"},
	}
	securityContext.ReadOnlyRootFilesystem = ptr.To(true)
})

var GetDefaultRestrictedContainerSecurityContext = func(seccompType string, applyContainerDefaults ApplyContainerSecurityDefaults) *corev1.SecurityContext {
	// Use default value if not set
	if seccompType == "" {
		seccompType = "RuntimeDefault"
	}

	defaultRestrictedContainerSecurityContext := &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileType(seccompType),
		},
	}
	applyContainerDefaults(defaultRestrictedContainerSecurityContext)
	return defaultRestrictedContainerSecurityContext
}

var DefaultOverrides = map[string]map[string]ApplyContainerSecurityDefaults{
	"gloo": {
		"gloo":          ApplyDiscoverySecurityDefaults,
		"envoy-sidecar": ApplyRunAsUserSecurityDefaults,
		"sds":           ApplyRunAsUserSecurityDefaults,
	},
	"discovery":                     {"discovery": ApplyDiscoverySecurityDefaults},
	"gateway-proxy":                 {"gateway-proxy": ApplyDiscoverySecurityDefaults},
	"gloo-mtls-certgen":             {"certgen": ApplyRunAsUserSecurityDefaults},
	"gloo-resource-cleanup":         {"kubectl": ApplyRunAsUserSecurityDefaults},
	"gloo-resource-migration":       {"kubectl": ApplyRunAsUserSecurityDefaults},
	"gloo-resource-rollout-check":   {"kubectl": ApplyRunAsUserSecurityDefaults},
	"gloo-resource-rollout-cleanup": {"kubectl": ApplyRunAsUserSecurityDefaults},
	"gloo-resource-rollout":         {"kubectl": ApplyRunAsUserSecurityDefaults},
	"prometheus-server-migration":   {"prometheus-server-migration": ApplyRunAsUserSecurityDefaults},
	"gateway-certgen":               {"certgen": ApplyRunAsUserSecurityDefaults},
	"ingress-proxy":                 {"ingress-proxy": ApplyKnativeSecurityDefaults},
	"clusteringress-proxy":          {"clusteringress-proxy": ApplyClusterIngressSecurityDefaults},
	"knative-external-proxy":        {"knative-external-proxy": ApplyKnativeSecurityDefaults},
	"knative-internal-proxy":        {"knative-internal-proxy": ApplyKnativeSecurityDefaults},
	"gloo-mtls-certgen-cronjob":     {"certgen": ApplyRunAsUserSecurityDefaults},
	"gateway-certgen-cronjob":       {"certgen": ApplyRunAsUserSecurityDefaults},
}

func FilterAndValidateSecurityContexts(testManifest TestManifest, validateContainer func(container corev1.Container, resourceName string), filter func(resource *unstructured.Unstructured) bool) int {
	foundContainers := 0

	testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
		return resource.GetKind() == "Deployment" || resource.GetKind() == "Job" || resource.GetKind() == "CronJob"
	}).ExpectAll(func(resource *unstructured.Unstructured) {

		// Get the pods and validate their security context
		var containers []corev1.Container
		resourceUncast, err := kuberesource.ConvertUnstructured(resource)
		Expect(err).NotTo(HaveOccurred())

		switch resource.GetKind() {
		case "Deployment":
			deployment := resourceUncast.(*appsv1.Deployment)
			containers = deployment.Spec.Template.Spec.Containers
		case "Job":
			job := resourceUncast.(*batchv1.Job)
			containers = job.Spec.Template.Spec.Containers
		case "CronJob":
			job := resourceUncast.(*batchv1.CronJob)
			containers = job.Spec.JobTemplate.Spec.Template.Spec.Containers
		default:
			Fail(fmt.Sprintf("Unexpected resource kind: %s", resource.GetKind()))
		}

		for _, container := range containers {
			// Uncomment this to print the enumerated list of containers
			// fmt.Printf("%s, %s, %s\n", resource.GetKind(), resource.GetName(), container.Name)
			foundContainers += 1
			validateContainer(container, resource.GetName())
		}
	})

	return foundContainers
}

func ValidateSecurityContexts(testManifest TestManifest, validateContainer func(container corev1.Container, resourceName string)) int {
	return FilterAndValidateSecurityContexts(testManifest, validateContainer, func(resource *unstructured.Unstructured) bool {
		return resource.GetKind() == "Deployment" || resource.GetKind() == "Job" || resource.GetKind() == "CronJob"
	})
}

const ExpectedContainers = 21

// Deployment, gloo, envoy-sidecar
// Deployment, gloo, sds
// Deployment, gloo, gloo
// Deployment, ingress, ingress
// Deployment, ingress-proxy, ingress-proxy
// Deployment, knative-external-proxy, knative-external-proxy
// Deployment, knative-internal-proxy, knative-internal-proxy
// Deployment, discovery, discovery
// Deployment, gateway-proxy-access-logger, access-logger
// Deployment, gateway-proxy, gateway-proxy
// Deployment, gateway-proxy, sds
// Deployment, gateway-proxy, istio-proxy
// Job, gloo-resource-rollout, kubectl
// CronJob, gloo-mtls-certgen-cronjob, certgen
// CronJob, gateway-certgen-cronjob, certgen
// Job, gloo-mtls-certgen, certgen
// Job, gloo-resource-cleanup, kubectl
// Job, gloo-resource-migration, kubectl
// Job, gloo-resource-rollout-check, kubectl
// Job, gloo-resource-rollout-cleanup, kubectl
// Job, gateway-certgen, certgen

var ContainerSecurityContextRoots = []string{
	"accessLogger.accessLoggerContainerSecurityContext",
	"discovery.deployment.discoveryContainerSecurityContext",
	"gateway.certGenJob.containerSecurityContext",
	"gatewayProxies.gatewayProxy.podTemplate.glooContainerSecurityContext",
	"global.glooMtls.envoy.securityContext",
	"global.glooMtls.istioProxy.securityContext",
	"global.glooMtls.sds.securityContext",
	"gloo.deployment.glooContainerSecurityContext",
	"ingress.deployment.ingressContainerSecurityContext",
	"ingressProxy.deployment.ingressProxyContainerSecurityContext",
	"settings.integrations.knative.proxy.containerSecurityContext",
}
