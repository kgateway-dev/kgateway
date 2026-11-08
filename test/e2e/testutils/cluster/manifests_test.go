//go:build e2e

package cluster

import (
	"testing"

	kubelib "istio.io/istio/pkg/kube"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
)

var configMapGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

func TestManifestsDelete(t *testing.T) {
	g := NewWithT(t)

	// The fake client's typed and dynamic clients do not share state, and Manifests deletes through the
	// dynamic client, so seed and inspect objects through it.
	client := kubelib.NewFakeClient()
	cms := client.Dynamic().Resource(configMapGVR)
	for _, nn := range [][2]string{{"other", "named"}, {"target", "unnamespaced"}, {"target", "kept"}} {
		cm := &unstructured.Unstructured{}
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")
		cm.SetNamespace(nn[0])
		cm.SetName(nn[1])
		_, err := cms.Namespace(nn[0]).Create(t.Context(), cm, metav1.CreateOptions{})
		g.Expect(err).NotTo(HaveOccurred())
	}

	err := Manifests{client: client}.InNamespace("target").Delete(t.Context(), `apiVersion: v1
kind: ConfigMap
metadata:
  name: unnamespaced
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: missing
---
`)
	g.Expect(err).NotTo(HaveOccurred(), "a missing object is not an error")
	_, err = cms.Namespace("target").Get(t.Context(), "unnamespaced", metav1.GetOptions{})
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "an object without metadata.namespace is deleted from InNamespace's namespace")
	_, err = cms.Namespace("target").Get(t.Context(), "kept", metav1.GetOptions{})
	g.Expect(err).NotTo(HaveOccurred(), "objects not in the manifest are left alone")

	named := `apiVersion: v1
kind: ConfigMap
metadata:
  name: named
  namespace: other
`
	err = Manifests{client: client}.InNamespace("target").Delete(t.Context(), named)
	g.Expect(err).To(MatchError(ContainSubstring(`provided namespace "other" but apply called with "target"`)),
		"a manifest namespace that conflicts with InNamespace is rejected")

	err = Manifests{client: client}.WithoutWait().Delete(t.Context(), named)
	g.Expect(err).NotTo(HaveOccurred())
	_, err = cms.Namespace("other").Get(t.Context(), "named", metav1.GetOptions{})
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the manifest's own namespace is used when InNamespace is unset")
}
