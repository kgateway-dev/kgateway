package endpointswatcher_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/gogo/protobuf/types"
	"k8s.io/apimachinery/pkg/labels"
	"time"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/solo-io/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/hashicorp/consul/api"
	"github.com/solo-io/gloo/test/helpers"
	"strings"
	"github.com/solo-io/gloo/pkg/log"
	. "github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
)

var upstreams []*v1.Upstream

var _ = Describe("EndpointsWatcher", func() {
	eds := NewEndpointsWatcher(opts, &kubernetes.Plugin{}, &consul.Plugin{})
	Context("running both consul eds and kubernetes eds", func() {
		BeforeEach(func() {
			createKubeResources()
			createConsulResources()
		})
		It("aggregates the endpoints for all the upstreams", func() {
			log.Printf("test begin")
			go eds.Run(make(chan struct{}))
			go eds.TrackUpstreams(upstreams)
			time.Sleep(time.Second)
			var endpoints endpointdiscovery.EndpointGroups
			Eventually(func() (endpointdiscovery.EndpointGroups, error) {
				log.Printf("trying")
				select {
				case err := <-eds.Error():
					return nil, err
				case endpoints = <-eds.Endpoints():
				case <-time.After(time.Second):
				}
				return endpoints, nil
			}).Should(Equal(endpointdiscovery.EndpointGroups{
				"upstream-for-svc3-a": []endpointdiscovery.Endpoint{
					{Address: "5.6.7.8", Port: 3456},
					{Address: "6.7.8.9", Port: 3456},
				},
				"upstream-for-svc1": []endpointdiscovery.Endpoint{
				{Address: "1.2.3.4", Port: 1234},
				{Address: "2.3.4.5", Port: 2345},
			},
				"upstream-for-svc2": []endpointdiscovery.Endpoint{
			{Address: "3.4.5.6", Port: 3456},
			},
				"upstream-for-svc3-a-b":[]endpointdiscovery.Endpoint{
			{Address: "6.7.8.9", Port: 3456},
			},
				"upstream-for-svc3": []endpointdiscovery.Endpoint{
			{Address: "4.5.6.7", Port: 3456},
			{Address: "5.6.7.8", Port: 3456},
			{Address: "6.7.8.9", Port: 3456},
				},
			}))
		})
	})
})

func createKubeResources() {
	specLabels := map[string]string{"version": "v1"}
	singlePortServiceSpec := kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
		ServiceName:      "test-service",
		ServiceNamespace: namespace,
	})
	serviceWithSpecificPortSpec := kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
		ServiceName:      "test-service-with-port",
		ServiceNamespace: namespace,
		ServicePort:      80,
	})
	serviceWithSpecificLabels := kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
		ServiceName:      "test-service-with-labels",
		ServiceNamespace: namespace,
		ServicePort:      80,
		Labels:           specLabels,
	})
	kubeUpstreams := []*v1.Upstream{
		{
			Name: "my_upstream_with_specific_port",
			Type: kubernetes.UpstreamTypeKube,
			Spec: serviceWithSpecificPortSpec,
		},
		{
			Name: "my_upstream_with_one_port",
			Type: kubernetes.UpstreamTypeKube,
			Spec: singlePortServiceSpec,
		},
		{
			Name: "my_upstream_with_specific_port_v1",
			Type: kubernetes.UpstreamTypeKube,
			Spec: serviceWithSpecificLabels,
		},
	}
	for _, us := range kubeUpstreams {
		serviceName := us.Spec.Fields["service_name"].Kind.(*types.Value_StringValue).StringValue
		podLabels := map[string]string{"app": serviceName}
		withoutLabels := &kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-for-" + serviceName,
				Namespace: namespace,
				Labels:    podLabels,
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
		}
		_, err := kubeClient.CoreV1().Pods(namespace).Create(withoutLabels)
		Expect(err).NotTo(HaveOccurred())
		withLabels := &kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-withlabels-for-" + serviceName,
				Namespace: namespace,
				Labels:    labels.Merge(podLabels, specLabels),
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "nginx",
						Image: "nginx:latest",
					},
				},
			},
		}
		p, err := kubeClient.CoreV1().Pods(namespace).Create(withLabels)
		Expect(err).NotTo(HaveOccurred())

		// wait for pod to be running
		podReady := make(chan struct{})
		go func() {
			for {
				pod, err := kubeClient.CoreV1().Pods(namespace).Get(p.Name, metav1.GetOptions{})
				if err != nil {
					panic(err)
				}
				if pod.Status.Phase == kubev1.PodRunning {
					close(podReady)
					return
				}
				time.Sleep(time.Second)
			}
		}()

		select {
		case <-time.After(time.Minute):
			Fail("timed out waiting for pod " + p.Name + " to start")
		case <-podReady:
		}

		service := &kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: namespace,
			},
			Spec: kubev1.ServiceSpec{
				Selector: podLabels,
				Ports: []kubev1.ServicePort{
					{
						Name: "foo",
						Port: 8080,
					},
				},
			},
		}
		_, err = kubeClient.CoreV1().Services(namespace).Create(service)
		Expect(err).NotTo(HaveOccurred())
	}
	upstreams = append(upstreams, kubeUpstreams...)
}
func createConsulResources() {
	cfg := api.DefaultConfig()

	consul, err := api.NewClient(cfg)
	Expect(err).NotTo(HaveOccurred())

	svc1a := newConsulSvc("svc1", nil, "1.2.3.4", 1234)
	svc1b := newConsulSvc("svc1", nil, "2.3.4.5", 2345)
	svc2 := newConsulSvc("svc2", nil, "3.4.5.6", 3456)
	svc3notags := newConsulSvc("svc3", []string{}, "4.5.6.7", 3456)
	svc3sometags := newConsulSvc("svc3", []string{"a"}, "5.6.7.8", 3456)
	svc3alltags := newConsulSvc("svc3", []string{"a", "b"}, "6.7.8.9", 3456)

	err = consul.Agent().ServiceRegister(svc1a)
	Expect(err).NotTo(HaveOccurred())
	err = consul.Agent().ServiceRegister(svc1b)
	Expect(err).NotTo(HaveOccurred())
	err = consul.Agent().ServiceRegister(svc2)
	Expect(err).NotTo(HaveOccurred())
	err = consul.Agent().ServiceRegister(svc3notags)
	Expect(err).NotTo(HaveOccurred())
	err = consul.Agent().ServiceRegister(svc3sometags)
	Expect(err).NotTo(HaveOccurred())
	err = consul.Agent().ServiceRegister(svc3alltags)
	Expect(err).NotTo(HaveOccurred())

	consulUpstreams := []*v1.Upstream{
		newUpstreamFromSvc(svc1a),
		newUpstreamFromSvc(svc2),
		newUpstreamFromSvc(svc3notags),
		newUpstreamFromSvc(svc3sometags),
		newUpstreamFromSvc(svc3alltags),
	}

	upstreams = append(upstreams, consulUpstreams...)
}

func newConsulSvc(name string, tags []string, address string, port int) *api.AgentServiceRegistration {
	return &api.AgentServiceRegistration{
		ID:      helpers.RandString(4),
		Name:    name,
		Tags:    tags,
		Port:    port,
		Address: address,
	}
}

func newUpstreamFromSvc(svc *api.AgentServiceRegistration) *v1.Upstream {
	name := "upstream-for-" + svc.Name
	if len(svc.Tags) > 0 {
		name += "-" + strings.Join(svc.Tags, "-")
	}
	return &v1.Upstream{
		Name: name,
		Type: consul.UpstreamTypeConsul,
		Spec: consul.EncodeUpstreamSpec(consul.UpstreamSpec{
			ServiceName: svc.Name,
			ServiceTags: svc.Tags,
		}),
	}
}
