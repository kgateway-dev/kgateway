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

	manifestVhoRemoveXBar      = filepath.Join(util.MustGetThisDir(), "testdata", "vho-remove-x-bar.yaml")
	manifestVhoSectionAddXFoo  = filepath.Join(util.MustGetThisDir(), "testdata", "vho-section-add-x-foo.yaml")
	manifestVhoRemoveXBaz      = filepath.Join(util.MustGetThisDir(), "testdata", "vho-remove-x-baz.yaml")
	manifestVhoWebhookReject   = filepath.Join(util.MustGetThisDir(), "testdata", "vho-webhook-reject.yaml")
	manifestVhoMergeRemoveXBaz = filepath.Join(util.MustGetThisDir(), "testdata", "vho-merge-remove-x-baz.yaml")

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

	// VHO to add a x-foo header
	vhoRemoveXBar = metav1.ObjectMeta{
		Name:      "remove-x-bar-header",
		Namespace: "default",
	}
	// VHO to remove a x-baz header
	vhoRemoveXBaz = metav1.ObjectMeta{
		Name:      "remove-x-baz-header",
		Namespace: "default",
	}
	// VHO to remove a x-baz header
	vhoMergeRemoveXBaz = metav1.ObjectMeta{
		Name:      "remove-x-baz-merge",
		Namespace: "default",
	}
	// VHO to add a x-foo header in a section
	vhoSectionAddXFoo = metav1.ObjectMeta{
		Name:      "add-x-foo-header",
		Namespace: "default",
	}
	// VHO that should be rejected by the validating webhook
	vhoWebhookReject = metav1.ObjectMeta{
		Name:      "bad-retries",
		Namespace: "default",
	}

	// Expects a 200 response with x-bar and x-baz headers
	defaultResponse = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom: gomega.And(
			gomega.Not(matchers.ContainHeaderKeys([]string{"x-foo"})),
			matchers.ContainHeaderKeys([]string{"x-bar", "x-baz"}),
		),
		Body: gstruct.Ignore(),
	}

	// Expects default response with no x-bar header
	expectedResponseWithoutXBar = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom: gomega.And(
			gomega.Not(matchers.ContainHeaderKeys([]string{"x-bar"})),
			matchers.ContainHeaderKeys([]string{"x-baz"}),
		),
		Body: gstruct.Ignore(),
	}

	// Expects default response with no x-baz header
	expectedResponseWithoutXBaz = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Custom: gomega.And(
			matchers.ContainHeaderKeys([]string{"x-bar"}),
			gomega.Not(matchers.ContainHeaderKeys([]string{"x-baz"})),
		),
		Body: gstruct.Ignore(),
	}

	// Expects default response with x-foo header
	expectedResponseWithXFoo = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]interface{}{
			"x-foo": gomega.Equal("foo"),
		},
		// Make sure the x-bar isn't being removed as a function of the unwanted VHO
		Custom: gomega.And(
			matchers.ContainHeaderKeys([]string{"x-foo", "x-bar", "x-baz"}),
		),
		Body: gstruct.Ignore(),
	}
)
