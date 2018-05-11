package snapshot_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "github.com/solo-io/gloo/internal/control-plane/snapshot"
	"github.com/solo-io/gloo/pkg/bootstrap/configstorage"
	secretwatchersetup "github.com/solo-io/gloo/pkg/bootstrap/secretwatcher"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/internal/control-plane/configwatcher"
	"github.com/solo-io/gloo/pkg/bootstrap/artifactstorage"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/internal/control-plane/endpointswatcher"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"path/filepath"
	"os"
	"github.com/solo-io/gloo/test/helpers"
	"k8s.io/client-go/tools/clientcmd"
	kube "k8s.io/client-go/kubernetes"
	"github.com/gogo/protobuf/types"
	"k8s.io/apimachinery/pkg/labels"
	"time"
	"log"
	kubeupstreamdiscovery "github.com/solo-io/gloo/internal/upstream-discovery/kube"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
)

var (
	store storage.Interface

	namespace  string
	kubeClient kube.Interface

	storageOpts = bootstrap.StorageOptions{
		Type: bootstrap.WatcherTypeKube,
	}

	opts bootstrap.Options
)

var _ = Describe("Emitter", func() {
	Describe("Snapshot()", func() {
		Context("using kubernetes for config, endpoints, secrets, and files", func() {
			if os.Getenv("RUN_KUBE_TESTS") != "1" {
				log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
				return
			}
			BeforeEach(func() {
				namespace = helpers.RandString(8)
				err := helpers.SetupKubeForTest(namespace)
				helpers.Must(err)

				opts = bootstrap.Options{
					ConfigStorageOptions: storageOpts,
					SecretStorageOptions: storageOpts,
					FileStorageOptions:   storageOpts,
					KubeOptions: bootstrap.KubeOptions{
						Namespace:  namespace,
						KubeConfig: filepath.Join(os.Getenv("HOME"), ".kube", "config"),
					},
				}

				cfg, err := clientcmd.BuildConfigFromFlags(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
				Expect(err).NotTo(HaveOccurred())

				// add a pod and service pointing to it
				kubeClient, err = kube.NewForConfig(cfg)
				Expect(err).NotTo(HaveOccurred())

				store, err = configstorage.Bootstrap(opts)
				Expect(err).NotTo(HaveOccurred())

				store.V1().VirtualServices().Create(&v1.VirtualService{
					Name: "foo",
				})

				discovery, err := kubeupstreamdiscovery.NewUpstreamController(cfg, store, time.Minute)
				Expect(err).NotTo(HaveOccurred())
				go discovery.Run(make(chan struct{}))

				createKubeResources()
			})
			AfterEach(func() {
				helpers.TeardownKube(namespace)
			})
			It("sends snapshots down the channel", func() {
				cfgWatcher, err := configwatcher.NewConfigWatcher(store)
				Expect(err).NotTo(HaveOccurred())
				secretWatcher, err := secretwatchersetup.Bootstrap(opts)
				filestore, err := artifactstorage.Bootstrap(opts)
				Expect(err).NotTo(HaveOccurred())
				fileWatcher, err := filewatcher.NewFileWatcher(filestore)
				Expect(err).NotTo(HaveOccurred())
				endpointsWatcher := endpointswatcher.NewEndpointsWatcher(opts, &kubernetes.Plugin{})
				getDependencies := func(cfg *v1.Config) []*plugins.Dependencies {
					return []*plugins.Dependencies{
						{
							SecretRefs: []string{"secret-name"},
							FileRefs:   []string{"myfile:username"},
						},
					}
				}
				emitter := NewEmitter(cfgWatcher, secretWatcher, fileWatcher, endpointsWatcher, getDependencies)
				go emitter.Run(make(chan struct{}))
				var snap *Cache
				Eventually(func() (*Cache, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
					case <-time.After(time.Second):
					}
					return snap, nil
				}, time.Second*5, time.Millisecond*250).ShouldNot(BeNil())
				files := snap.Files
				Eventually(func() (filewatcher.Files, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						files = snap.Files
					case <-time.After(time.Second):
					}
					for _, f := range files {
						f.ResourceVersion = ""
					}
					return files, nil
				}, time.Second*5, time.Millisecond*250).Should(Equal(filewatcher.Files{
					"myfile:username": {
						Ref:      "myfile:username",
						Contents: []byte("me@example.com"),
					},
				}))
				secrets := snap.Secrets
				Eventually(func() (secretwatcher.SecretMap, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						secrets = snap.Secrets
					case <-time.After(time.Second):
					}
					for _, s := range secrets {
						s.ResourceVersion = ""
					}
					return secrets, nil
				}, time.Second*5, time.Millisecond*250).Should(Equal(secretwatcher.SecretMap{
					"secret-name": {
						Ref: "secret-name",
						Data: map[string]string{
							"username": "me@example.com",
							"password": "foobar",
						},
					},
				}))
				endpoints := snap.Endpoints
				Eventually(func() (endpointdiscovery.EndpointGroups, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						endpoints = snap.Endpoints
					case <-time.After(time.Second):
					}
					for _, s := range secrets {
						s.ResourceVersion = ""
					}
					return endpoints, nil
				}, time.Second*5, time.Millisecond*250).Should(And(
					HaveKey(namespace+"-test-service-8080"),
					HaveKey(namespace+"-test-service-with-labels-8080"),
					HaveKey(namespace+"-test-service-with-port-8080"),
				))
				Eventually(func() ([]endpointdiscovery.Endpoint, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						endpoints = snap.Endpoints
					case <-time.After(time.Second):
					}
					for _, s := range secrets {
						s.ResourceVersion = ""
					}
					if endpoints == nil {
						return nil, errors.New("timed out")
					}
					return endpoints[namespace+"-test-service-8080"], nil
				}, time.Second*5, time.Millisecond*250).Should(HaveLen(2))
				Eventually(func() ([]endpointdiscovery.Endpoint, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						endpoints = snap.Endpoints
					case <-time.After(time.Second):
					}
					for _, s := range secrets {
						s.ResourceVersion = ""
					}
					if endpoints == nil {
						return nil, errors.New("timed out")
					}
					return endpoints[namespace+"-test-service-with-labels-8080"], nil
				}, time.Second*5, time.Millisecond*250).Should(HaveLen(2))
				Eventually(func() ([]endpointdiscovery.Endpoint, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						endpoints = snap.Endpoints
					case <-time.After(time.Second):
					}
					for _, s := range secrets {
						s.ResourceVersion = ""
					}
					if endpoints == nil {
						return nil, errors.New("timed out")
					}
					return endpoints[namespace+"-test-service-with-port-8080"], nil
				}, time.Second*5, time.Millisecond*250).Should(HaveLen(2))
				cfg := snap.Cfg
				Eventually(func() (*v1.Config, error) {
					select {
					case err := <-emitter.Error():
						return nil, err
					case snap = <-emitter.Snapshot():
						cfg = snap.Cfg
					case <-time.After(time.Second):
					}
					if cfg != nil {
						for _, u := range cfg.Upstreams {
							u.Metadata.ResourceVersion = ""
						}
						for _, v := range cfg.VirtualServices {
							v.Metadata.ResourceVersion = ""
						}
					}
					return cfg, nil
				}, time.Second*5, time.Millisecond*250).Should(Equal(&v1.Config{
					Upstreams: []*v1.Upstream{
						{
							Name:              namespace + "-test-service-8080",
							Type:              "kubernetes",
							ConnectionTimeout: 0,
							Spec: &types.Struct{
								Fields: map[string]*types.Value{
									"service_name": {
										Kind: &types.Value_StringValue{
											StringValue: "test-service",
										},
									},
									"service_namespace": {
										Kind: &types.Value_StringValue{
											StringValue: namespace,
										},
									},
									"service_port": {
										Kind: &types.Value_NumberValue{NumberValue: 8080},
									},
								},
							},
							Functions:   nil,
							ServiceInfo: nil,
							Status:      nil,
							Metadata: &v1.Metadata{
								Namespace: namespace,
								Annotations: map[string]string{
									"generated_by": "kubernetes-upstream-discovery",
								},
							},
						},
						{
							Name:              namespace + "-test-service-with-labels-8080",
							Type:              "kubernetes",
							ConnectionTimeout: 0,
							Spec: &types.Struct{
								Fields: map[string]*types.Value{
									"service_name": {
										Kind: &types.Value_StringValue{
											StringValue: "test-service-with-labels",
										},
									},
									"service_namespace": {
										Kind: &types.Value_StringValue{
											StringValue: namespace,
										},
									},
									"service_port": {
										Kind: &types.Value_NumberValue{NumberValue: 8080},
									},
								},
							},
							Functions:   nil,
							ServiceInfo: nil,
							Status:      nil,
							Metadata: &v1.Metadata{
								Namespace: namespace,
								Annotations: map[string]string{
									"generated_by": "kubernetes-upstream-discovery",
								},
							},
						},
						{
							Name:              namespace + "-test-service-with-port-8080",
							Type:              "kubernetes",
							ConnectionTimeout: 0,
							Spec: &types.Struct{
								Fields: map[string]*types.Value{
									"service_name": {
										Kind: &types.Value_StringValue{
											StringValue: "test-service-with-port",
										},
									},
									"service_namespace": {
										Kind: &types.Value_StringValue{
											StringValue: namespace,
										},
									},
									"service_port": {
										Kind: &types.Value_NumberValue{NumberValue: 8080},
									},
								},
							},
							Functions:   nil,
							ServiceInfo: nil,
							Status:      nil,
							Metadata: &v1.Metadata{
								Namespace: namespace,
								Annotations: map[string]string{
									"generated_by": "kubernetes-upstream-discovery",
								},
							},
						},
					},
					VirtualServices: []*v1.VirtualService{
						{
							Name:      "foo",
							Metadata: &v1.Metadata{
								Namespace: namespace,
							},
						},
					},
				}))
			})
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

	secret := &kubev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-name",
			Namespace: namespace,
		},
		Data: map[string][]byte{"username": []byte("me@example.com"), "password": []byte("foobar")},
	}

	_, err := kubeClient.CoreV1().Secrets(namespace).Create(secret)
	Expect(err).NotTo(HaveOccurred())

	file := &kubev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myfile",
			Namespace: namespace,
		},
		Data: map[string]string{"username": "me@example.com"},
	}

	_, err = kubeClient.CoreV1().ConfigMaps(namespace).Create(file)
	Expect(err).NotTo(HaveOccurred())
}
