package deployer_test

import (
	"context"
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/deployer"
)

func convertUnstructured[T any](f client.Object) T {
	jsonBytes, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}

	// Create an empty ClusterRole object
	var ret T

	// Unmarshal the JSON into the ClusterRole object
	if err := json.Unmarshal(jsonBytes, &ret); err != nil {
		panic(err)
	}
	return ret
}

func findGvkInRules(cr rbacv1.ClusterRole, gvk schema.GroupVersionKind) bool {
	for _, rule := range cr.Rules {
		for _, apiGroup := range rule.APIGroups {
			if apiGroup == gvk.Group {
				for _, resource := range rule.Resources {
					if strings.Contains(resource, strings.ToLower(gvk.Kind)) {
						return true
					}
				}
			}
		}
	}
	return false
}

var _ = Describe("Deployer", func() {
	var (
		d *deployer.Deployer
	)
	BeforeEach(func() {
		var err error
		d, err = deployer.NewDeployer(controller.NewScheme(), "gloo-gateway2", "foo", "xds", 8080)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should get gvks", func() {
		gvks, err := d.GetGvksToWatch(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(gvks).NotTo(BeEmpty())
	})

	It("rbac should have our gvks", func() {
		gvks, err := d.GetGvksToWatch(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// render the control plane chart
		vals := map[string]any{
			"controlPlane": map[string]any{"enabled": true},
			"gateway": map[string]any{
				"enabled":       false,
				"createGateway": false,
			},
		}
		cpObjs, err := d.Render(context.Background(), "default", vals)
		Expect(err).NotTo(HaveOccurred())

		// find the rbac role with deploy in its name
		for _, obj := range cpObjs {
			if obj.GetObjectKind().GroupVersionKind().Kind == "ClusterRole" {
				if strings.Contains(obj.GetName(), "deploy") {
					cr := convertUnstructured[rbacv1.ClusterRole](obj)
					for _, gvk := range gvks {
						Expect(findGvkInRules(cr, gvk)).To(BeTrue(), "gvk %v not found in rules", gvk)
					}
				}
			}
		}

	})

	It("should not fail with no ports", func() {
		gw := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
		}

		objs, err := d.GetObjsToDeploy(context.Background(), gw)

		Expect(err).NotTo(HaveOccurred())
		Expect(objs).NotTo(BeEmpty())
	})

	It("should get objects with owner refs", func() {
		gw := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
			Spec: api.GatewaySpec{
				Listeners: []api.Listener{
					{
						Name: "listener-1",
						Port: 8080,
					},
				},
			},
		}

		objs, err := d.GetObjsToDeploy(context.Background(), gw)

		Expect(err).NotTo(HaveOccurred())
		Expect(objs).NotTo(BeEmpty())

		for _, obj := range objs {
			ownerRefs := obj.GetOwnerReferences()
			Expect(ownerRefs).To(HaveLen(1))
			Expect(ownerRefs[0].Name).To(Equal(gw.Name))
			Expect(ownerRefs[0].UID).To(Equal(gw.UID))
			Expect(ownerRefs[0].Kind).To(Equal(gw.Kind))
			Expect(ownerRefs[0].APIVersion).To(Equal(gw.APIVersion))
			Expect(*ownerRefs[0].Controller).To(Equal(true))
		}
	})

	It("should config map with valid envoy yaml", func() {
		gw := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
		}

		objs, err := d.GetObjsToDeploy(context.Background(), gw)

		Expect(err).NotTo(HaveOccurred())
		Expect(objs).NotTo(BeEmpty())

		envoyYaml := getEnvoyConfig(objs)
		Expect(envoyYaml).NotTo(BeEmpty())

		// make sure it's valid yaml
		var envoyConfig map[string]any

		err = yaml.Unmarshal([]byte(envoyYaml), &envoyConfig)
		Expect(err).NotTo(HaveOccurred(), "envoy config is not valid yaml: %s", envoyYaml)

	})

	It("support segmenting by release", func() {

		d1, err := deployer.NewDeployer(controller.NewScheme(), "r1", "foo", "xds", 8080)
		Expect(err).NotTo(HaveOccurred())

		d2, err := deployer.NewDeployer(controller.NewScheme(), "", "foo", "xds", 8080)
		Expect(err).NotTo(HaveOccurred())

		gw := &api.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "default",
				UID:       "1235",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Gateway",
				APIVersion: "gateway.solo.io/v1beta1",
			},
		}

		objs1, err := d1.GetObjsToDeploy(context.Background(), gw)
		Expect(err).NotTo(HaveOccurred())
		Expect(objs1).NotTo(BeEmpty())
		objs2, err := d2.GetObjsToDeploy(context.Background(), gw)
		Expect(err).NotTo(HaveOccurred())
		Expect(objs2).NotTo(BeEmpty())

		for _, obj := range objs1 {
			Expect(obj.GetName()).To(Equal("r1-foo-dp"))
		}
		for _, obj := range objs2 {
			Expect(obj.GetName()).To(Equal("foo-dp"))
		}

	})

})

func getEnvoyConfig(objs []client.Object) string {
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
			cm := convertUnstructured[corev1.ConfigMap](obj)
			envoyYaml := cm.Data["envoy.yaml"]
			if envoyYaml != "" {
				return envoyYaml
			}
		}
	}
	return ""
}
