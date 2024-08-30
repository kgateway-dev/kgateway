package virtualhost_options

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/gloo/test/gomega/matchers"
	e2edefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	setupManifests = []string{
		filepath.Join(util.MustGetThisDir(), "testdata", "setup.yaml"),
		e2edefaults.CurlPodManifest,
	}
	basicVhOManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "basic-vho.yaml")
	sectionNameVhOManifest = filepath.Join(util.MustGetThisDir(), "testdata", "section-name-vho.yaml")
	extraVhOManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "extra-vho.yaml")
	badVhOManifest         = filepath.Join(util.MustGetThisDir(), "testdata", "webhook-reject-bad-vho.yaml")
	extraVhOMergeManifest  = filepath.Join(util.MustGetThisDir(), "testdata", "extra-vho-merge.yaml")

	// When we apply the setup file, we expect resources to be created with this metadata
	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyService    = &corev1.Service{ObjectMeta: glooProxyObjectMeta}
	proxyDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gloo-proxy-gw",
			Namespace: "default",
		},
	}
	nginxPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
		},
	}
	exampleSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-svc",
			Namespace: "default",
		},
	}

	// VirtualHostOption resource to be created
	basicVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-length",
		Namespace: "default",
	}
	// Extra VirtualHostOption resource to be created
	extraVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "remove-content-type",
		Namespace: "default",
	}
	// Extra VirtualHostOption resource to be created to test merging of options
	extraMergeVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "extra-vho-merge",
		Namespace: "default",
	}
	// SectionName VirtualHostOption resource to be created
	sectionNameVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "add-foo-header",
		Namespace: "default",
	}
	// Bad VirtualHostOption resource to be created
	badVirtualHostOptionMeta = metav1.ObjectMeta{
		Name:      "bad-retries",
		Namespace: "default",
	}

	expectedResponseWithoutContentLength = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom:     gomega.Not(matchers.ContainHeaderKeys([]string{"content-length"})),
		Body:       gstruct.Ignore(),
	}

	expectedResponseWithoutContentType = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom:     gomega.Not(matchers.ContainHeaderKeys([]string{"content-type"})),
		Body:       gstruct.Ignore(),
	}

	expectedResponseWithFooHeader = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"foo": gomega.Equal("bar"),
		},
		// Make sure the content-length isn't being removed as a function of the unwanted VHO
		Custom: matchers.ContainHeaderKeys([]string{"content-length"}),
		Body:   gstruct.Ignore(),
	}
)
