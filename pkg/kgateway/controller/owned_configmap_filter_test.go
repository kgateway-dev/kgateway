package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// The Gateway reconciler watches ConfigMaps only to re-reconcile a Gateway when
// one of its child objects drifts, so ownedConfigMapFilter restricts that watch
// by label to objects the deployer rendered. Without the restriction the watch
// keeps a full typed copy of every ConfigMap in the cluster, which would cancel
// out the metadata-only cache in pkg/krtcollections/ondemand.
//
// The restriction is only safe while every rendered ConfigMap actually carries
// the label, which couples this filter to the Helm templates in
// pkg/kgateway/helm/envoy.
//
// Editing a template is already caught directly by TestRenderHelmChart, which
// diffs against these same goldens. This test guards the next step: someone
// regenerating the goldens to accept a label change would otherwise silently
// blind this watch, leaving drift unreconciled with nothing failing. It reads
// the goldens rather than rendering the chart, so it is that second link, not a
// replacement for the first.
func TestOwnedConfigMapFilterMatchesEveryRenderedConfigMap(t *testing.T) {
	root := repoRootForTest(t)
	goldens, err := filepath.Glob(filepath.Join(root, "test", "deployer", "testdata", "*-out.yaml"))
	if err != nil {
		t.Fatalf("globbing deployer goldens: %v", err)
	}
	if len(goldens) == 0 {
		t.Fatal("found no deployer goldens; this test is no longer checking anything")
	}

	selector := ownedConfigMapSelector
	if selector != wellknown.GatewayNameLabel {
		t.Fatalf("filter selects on %q; update this test to match", selector)
	}

	kindLine := regexp.MustCompile(`(?m)^kind: ConfigMap$`)
	nameLine := regexp.MustCompile(`(?m)^\s+name: (\S+)`)

	total := 0
	for _, path := range goldens {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for doc := range strings.SplitSeq(string(body), "\n---\n") {
			if !kindLine.MatchString(doc) {
				continue
			}
			total++
			if strings.Contains(doc, selector+":") {
				continue
			}
			name := "<unknown>"
			if m := nameLine.FindStringSubmatch(doc); m != nil {
				name = m[1]
			}
			t.Errorf("%s: rendered ConfigMap %q has no %s label, so the Gateway "+
				"reconciler's watch would not see it and drift on it would go unreconciled",
				filepath.Base(path), name, selector)
		}
	}
	if total == 0 {
		t.Fatal("deployer goldens contain no ConfigMaps; this test is no longer checking anything")
	}
	t.Logf("checked %d rendered ConfigMaps", total)
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}
