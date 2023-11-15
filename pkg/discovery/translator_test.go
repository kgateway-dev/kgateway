package discovery_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/discovery"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestYamls(t *testing.T) {

	ctx := context.Background()
	// get all folders in the testdata dir:
	testdata := filepath.Join(".", "testdata")

	dr, err := os.ReadDir(testdata)
	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())

	for _, d := range dr {
		if !d.IsDir() {
			continue
		}

		inputDir := filepath.Join(testdata, d.Name(), "inputs")
		outputF := filepath.Join(testdata, d.Name(), "output.yaml")
		output, err := os.ReadFile(outputF)
		useVip := false
		if err != nil && errors.Is(err, fs.ErrNotExist) {
			outputVipF := filepath.Join(testdata, d.Name(), "output_vip.yaml")
			output, err = os.ReadFile(outputVipF)
			if !errors.Is(err, fs.ErrNotExist) {
				g.Expect(err).NotTo(HaveOccurred())
			} else {
				t.Fatal(d.Name(), "output.yaml and output_vip.yaml do not exist")
			}
			useVip = true
		}

		objs, err := testutils.LoadFromFiles(ctx, inputDir)
		g.Expect(err).NotTo(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithScheme(scheme.NewScheme()).WithObjects(objs...).Build()

		t.Run(d.Name(), func(t *testing.T) {
			g := NewWithT(t)
			clusters, endpoints, upstreams := discovery.TranslateClusters(ctx, fakeClient, useVip)
			expected := toYaml(clusters, endpoints, upstreams)

			t.Log(d.Name() + "/output.yaml:\n" + expected)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(expected).To(Equal(string(output)))

		})

	}
}

func toYaml(clusters []*clusterv3.Cluster, endpoints []*endpointv3.ClusterLoadAssignment, warnings []string) string {
	jsonpbMarshaler := &protojson.MarshalOptions{UseProtoNames: true}
	// serialize as protojson
	var expected strings.Builder

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Name < clusters[j].Name
	})
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].ClusterName < endpoints[j].ClusterName
	})
	sort.StringSlice(warnings).Sort()

	for _, c := range clusters {
		jsn, err := jsonpbMarshaler.Marshal(c)
		if err != nil {
			panic(err)
		}
		yml, err := yaml.JSONToYAML(jsn)
		if err != nil {
			panic(err)
		}
		expected.Write(yml)
		expected.WriteString("---\n")
	}
	for _, c := range endpoints {
		jsn, err := jsonpbMarshaler.Marshal(c)
		if err != nil {
			panic(err)
		}
		yml, err := yaml.JSONToYAML(jsn)
		if err != nil {
			panic(err)
		}
		expected.Write(yml)
		expected.WriteString("---\n")
	}

	for _, c := range warnings {
		expected.WriteString(c)
		expected.WriteString("\n---\n")
	}

	return expected.String()
}
