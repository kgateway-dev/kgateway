//go:build e2e

package cluster

import (
	"context"
	"fmt"
	"os"
	"time"

	kubelib "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/test/util/yml"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	// manifestDeleteTimeout bounds how long a waiting delete blocks when the caller's context has no
	// earlier deadline.
	manifestDeleteTimeout = 2 * time.Minute
	// manifestDeletePollInterval is how often a waiting delete checks whether its objects are gone.
	manifestDeletePollInterval = 50 * time.Millisecond
)

// Manifests applies and deletes YAML manifests through the API server directly, without shelling out
// to kubectl. Apply is a server-side apply with the same field manager the kubectl helpers use, so
// the two can be mixed on the same objects. Delete ignores objects that are already gone and, unless
// WithoutWait is set, blocks until every deleted object has disappeared (as `kubectl delete` does).
//
// Apply and ApplyFiles take no context: the underlying client applies with context.Background(), so a
// test context does not cancel or bound an apply. Delete and DeleteFiles honor their context.
//
// A resource without metadata.namespace lands in the namespace set by InNamespace, or the client's
// default apply namespace ("default") when none is set; the kubeconfig's namespace is never used.
//
// Manifests is a value type: each With* method returns a modified copy.
type Manifests struct {
	client      kubelib.CLIClient
	namespace   string
	gracePeriod *int64
	noWait      bool
}

// Manifests returns a Manifests helper for this cluster.
func (c *Context) Manifests() Manifests {
	return Manifests{client: c.IstioClient}
}

// InNamespace sets the namespace for resources whose manifests omit metadata.namespace. A resource
// that names a different namespace is an error.
func (m Manifests) InNamespace(namespace string) Manifests {
	m.namespace = namespace
	return m
}

// WithGracePeriod sets the deletion grace period, in seconds.
func (m Manifests) WithGracePeriod(seconds int64) Manifests {
	m.gracePeriod = &seconds
	return m
}

// WithoutWait makes deletes return once the API server accepts them, without waiting for the
// objects to disappear.
func (m Manifests) WithoutWait() Manifests {
	m.noWait = true
	return m
}

// ApplyFiles server-side applies each file in order. Documents within a file are applied in order,
// so a file may declare a namespace ahead of the resources it contains.
func (m Manifests) ApplyFiles(files ...string) error {
	for _, f := range files {
		if err := m.client.ApplyYAMLFiles(m.namespace, f); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}

// Apply server-side applies each YAML document in order.
func (m Manifests) Apply(yamls ...string) error {
	for _, y := range yamls {
		for _, doc := range yml.SplitString(y) {
			if err := m.client.ApplyYAMLContents(m.namespace, doc); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteFiles deletes every object in the given files.
func (m Manifests) DeleteFiles(ctx context.Context, files ...string) error {
	yamls := make([]string, 0, len(files))
	for _, f := range files {
		d, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		yamls = append(yamls, string(d))
	}
	return m.Delete(ctx, yamls...)
}

// Delete deletes every object in the given YAML documents.
func (m Manifests) Delete(ctx context.Context, yamls ...string) error {
	type deleted struct {
		obj *unstructured.Unstructured
		dr  dynamic.ResourceInterface
	}
	var pending []deleted
	for _, y := range yamls {
		for _, doc := range yml.SplitString(y) {
			obj := &unstructured.Unstructured{}
			if err := sigsyaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
				return fmt.Errorf("decode manifest: %w", err)
			}
			if obj.GetKind() == "" {
				continue
			}
			dr, err := m.client.DynamicClientFor(obj.GroupVersionKind(), obj, m.namespace)
			if err != nil {
				return err
			}
			err = dr.Delete(ctx, obj.GetName(), metav1.DeleteOptions{GracePeriodSeconds: m.gracePeriod})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("delete %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
			pending = append(pending, deleted{obj: obj, dr: dr})
		}
	}
	if m.noWait {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, manifestDeleteTimeout)
	defer cancel()
	for _, p := range pending {
		err := wait.PollUntilContextCancel(ctx, manifestDeletePollInterval, true, func(ctx context.Context) (bool, error) {
			_, err := p.dr.Get(ctx, p.obj.GetName(), metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
		if err != nil {
			return fmt.Errorf("wait for %s %s to be deleted: %w", p.obj.GetKind(), p.obj.GetName(), err)
		}
	}
	return nil
}
