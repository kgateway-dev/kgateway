//go:build e2e

package base

import (
	"strings"
	"testing"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
)

func TestImagePullerPodName(t *testing.T) {
	testCases := []struct {
		name         string
		image        string
		expectedName string
	}{
		{
			name:         "digest image is sanitized and truncated",
			image:        "ricoli/hey@sha256:306dcd944a4398264f8a6bb43501afb3bb2285be4be248859bac971c57e3c270",
			expectedName: "image-puller-ricoli-hey-sha256-306dcd944a4398264f8-4e3783f8507c",
		},
		{
			name:         "mixed case and underscores are sanitized",
			image:        "Ghcr.io/Example/My_Image:V1.2.3",
			expectedName: "image-puller-ghcr-io-example-my-image-v1-2-3-1bb520d36c51",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			podName := imagePullerPodName(tt.image)
			if podName != tt.expectedName {
				t.Fatalf("pod name %q does not match expected %q", podName, tt.expectedName)
			}
			if len(podName) > imagePullerPodNameMaxLength {
				t.Fatalf("pod name %q exceeds %d characters", podName, imagePullerPodNameMaxLength)
			}

			if errs := apivalidation.NameIsDNSSubdomain(podName, false); len(errs) > 0 {
				t.Fatalf("pod name %q is not a valid DNS subdomain: %v", podName, errs)
			}
		})
	}
}

func TestImagePullerPodNameAvoidsSanitizationCollisions(t *testing.T) {
	first := imagePullerPodName("repo/my.image:1")
	second := imagePullerPodName("repo-my-image-1")

	if first == second {
		t.Fatalf("expected unique pod names for distinct images, got %q", first)
	}
}

func TestImageSet(t *testing.T) {
	images := newImageSet()
	if images.has("nginx:1.28.0") {
		t.Fatal("expected an empty set to not have nginx:1.28.0")
	}

	images.add("nginx:1.28.0")
	if !images.has("nginx:1.28.0") {
		t.Fatal("expected nginx:1.28.0 after adding it")
	}
	if images.has("nginx:1.29.0") {
		t.Fatal("expected a different tag to not be recorded")
	}
}

func TestSplitNamespaceResources(t *testing.T) {
	content := `apiVersion: v1
kind: Pod
metadata:
  name: heygw
  namespace: hey
---
# comment-only document
---
apiVersion: v1
kind: Namespace
metadata:
  name: hey
---
apiVersion: v1
kind: Service
metadata:
  name: example-svc
`
	namespaces, rest := splitNamespaceResources(t, content)

	if len(namespaces) != 1 || !strings.Contains(namespaces[0], "name: hey") {
		t.Fatalf("expected only the hey Namespace, got %q", namespaces)
	}
	if len(rest) != 2 || !strings.Contains(rest[0], "kind: Pod") || !strings.Contains(rest[1], "kind: Service") {
		t.Fatalf("expected the Pod and Service in order, got %q", rest)
	}
	if got := stripNamespaceResourcesFromContent(t, content); strings.Contains(got, "kind: Namespace") {
		t.Fatalf("stripped content still has a Namespace: %q", got)
	}
}
