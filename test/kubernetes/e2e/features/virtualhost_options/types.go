package virtualhost_options

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	targetRefManifest      = filepath.Join(util.MustGetThisDir(), "testdata", "header-manipulation-targetref.yaml")
	sectionNameVhOManifest = filepath.Join(util.MustGetThisDir(), "testdata", "section-name-vho.yaml")
	extraVhOManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "extra-vho.yaml")

	// When we apply the deployer-provision.yaml file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}

	// curlPod is the Pod that will be used to execute curl requests, and is defined in the fault injection manifest files
	curlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	// VirtualHostOption resource to be created
	virtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-length",
		Namespace: "default",
	}
	// Extra VirtualHostOption resource to be created
	extraVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-type",
		Namespace: "default",
	}
	// SectionName VirtualHostOption resource to be created
	sectionNameVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "add-foo-header",
		Namespace: "default",
	}

	expectedResponseWithoutContentLength = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom:     Not(matchers.ContainHeaderKeys([]string{"content-length"})),
		Body:       gstruct.Ignore(),
	}

	expectedResponseWithoutContentType = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom:     Not(matchers.ContainHeaderKeys([]string{"content-type"})),
		Body:       gstruct.Ignore(),
	}

	expectedResponseWithFooHeader = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"foo": Equal("bar"),
		},
		// Make sure the content-length isn't being removed as a function of the unwanted VHO
		Custom: matchers.ContainHeaderKeys([]string{"content-length"}),
		Body:   gstruct.Ignore(),
	}
)
