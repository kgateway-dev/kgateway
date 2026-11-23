package common

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// namespacesFromManifests parses the given manifest files and returns the names of every Namespace
// they declare. Documents that are not a named Namespace are ignored, so empty or malformed YAML
// documents are harmless.
func namespacesFromManifests(manifests ...string) ([]string, error) {
	var names []string
	for _, manifest := range manifests {
		data, err := os.ReadFile(manifest)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", manifest, err)
		}
		decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		for {
			obj := &unstructured.Unstructured{}
			if err := decoder.Decode(obj); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("decode manifest %s: %w", manifest, err)
			}
			if obj.GetObjectKind().GroupVersionKind().Kind == "Namespace" && obj.GetName() != "" {
				names = append(names, obj.GetName())
			}
		}
	}
	return names, nil
}

// terminatingNamespace returns the first of the named namespaces that still exists with a deletion
// timestamp, or "" if none is terminating. A missing or live namespace is fine to apply into.
func terminatingNamespace(ctx context.Context, c client.Client, names []string) (string, error) {
	for _, name := range names {
		ns := &corev1.Namespace{}
		err := c.Get(ctx, client.ObjectKey{Name: name}, ns)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("get namespace %s: %w", name, err)
		}
		if ns.DeletionTimestamp != nil {
			return name, nil
		}
	}
	return "", nil
}

// waitForTerminatingNamespaces blocks until none of the named namespaces is terminating, or ctx
// expires. It returns how long it waited, which is zero when nothing was terminating.
//
// Teardown deletes namespaces without waiting for them, because deletion takes close to a minute
// and nothing after a passing run needs them gone. The one thing that does is the next apply into
// the same namespace: a gotestsum rerun (a fresh process) or a back-to-back local run. A terminating
// namespace rejects new resources, so setup waits here instead, and only when it has to.
func waitForTerminatingNamespaces(ctx context.Context, c client.Client, names []string) (time.Duration, error) {
	if len(names) == 0 {
		return 0, nil
	}
	timeout := 3 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	start := time.Now()
	waited := false
	err := retry.UntilSuccess(func() error {
		name, err := terminatingNamespace(ctx, c, names)
		if err != nil {
			return err
		}
		if name != "" {
			waited = true
			return fmt.Errorf("namespace %s is still terminating", name)
		}
		return nil
	}, retry.Timeout(timeout), retry.BackoffDelay(50*time.Millisecond))
	if !waited {
		return 0, err
	}
	return time.Since(start), err
}
