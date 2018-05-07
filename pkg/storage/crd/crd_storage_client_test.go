package crd_test

import (
	"os"
	"path/filepath"

	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	. "github.com/solo-io/gloo/pkg/storage/crd"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	. "github.com/solo-io/gloo/test/helpers"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("CrdStorageClient", func() {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	var (
		masterUrl, kubeconfigPath string
		namespace                 string
		syncFreq                  = time.Minute
	)
	BeforeEach(func() {
		namespace = RandString(8)
		err := SetupKubeForTest(namespace)
		Must(err)
		kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		masterUrl = ""
	})
	AfterEach(func() {
		TeardownKube(namespace)
	})
	Describe("New", func() {
		It("creates a new client without error", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())
			_, err = NewStorage(cfg, namespace, syncFreq)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Describe("Register", func() {
		It("registers the crds", func() {
			cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())
			client, err := NewStorage(cfg, namespace, syncFreq)
			Expect(err).NotTo(HaveOccurred())
			err = client.V1().Register()
			Expect(err).NotTo(HaveOccurred())
			apiextClient, err := apiexts.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			crds, err := apiextClient.ApiextensionsV1beta1().CustomResourceDefinitions().List(metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			for _, crdSchema := range crdv1.KnownCRDs {
				var foundCrd *v1beta1.CustomResourceDefinition
				for _, crd := range crds.Items {
					if crd.Spec.Names.Kind == crdSchema.Kind {
						foundCrd = &crd
						break
					}
				}
				// if crd wasnt found, err
				Expect(foundCrd).NotTo(BeNil())

				Expect(foundCrd.Spec.Version).To(Equal(crdSchema.Version))
				Expect(foundCrd.Spec.Group).To(Equal(crdSchema.Group))
			}
		})
	})
	Describe("upstreams", func() {
		Describe("Create", func() {
			It("creates a crd from the item", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				upstream := NewTestUpstream1()
				createdUpstream, err := client.V1().Upstreams().Create(upstream)
				Expect(err).NotTo(HaveOccurred())
				upstream.Metadata = createdUpstream.GetMetadata()
				Expect(upstream).To(Equal(createdUpstream))
			})
		})
		Describe("Get", func() {
			It("gets a crd from the name", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				upstream := NewTestUpstream1()
				_, err = client.V1().Upstreams().Create(upstream)
				Expect(err).NotTo(HaveOccurred())
				created, err := client.V1().Upstreams().Get(upstream.Name)
				Expect(err).NotTo(HaveOccurred())
				upstream.Metadata = created.Metadata
				Expect(created).To(Equal(upstream))
			})
		})
		Describe("Update", func() {
			It("updates a crd from the item", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				upstream := NewTestUpstream1()
				created, err := client.V1().Upstreams().Create(upstream)
				Expect(err).NotTo(HaveOccurred())
				upstream.Metadata = created.GetMetadata()
				upstream.Type = "something-else"
				upstream.Metadata.Annotations["just_for_this_test"] = "bar"
				updated, err := client.V1().Upstreams().Update(upstream)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Metadata.Annotations).To(HaveKey("just_for_this_test"))
				upstream.Metadata = updated.GetMetadata()
				Expect(updated).To(Equal(upstream))
			})
		})
		Describe("Delete", func() {
			It("deletes a crd from the name", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				upstream := NewTestUpstream1()
				_, err = client.V1().Upstreams().Create(upstream)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Upstreams().Delete(upstream.Name)
				Expect(err).NotTo(HaveOccurred())
				_, err = client.V1().Upstreams().Get(upstream.Name)
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Describe("virtualservices", func() {
		Describe("Create", func() {
			It("creates a crd from the item", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				virtualService := NewTestVirtualService("something", NewTestRoute1())
				createdUpstream, err := client.V1().VirtualServices().Create(virtualService)
				Expect(err).NotTo(HaveOccurred())
				virtualService.Metadata = createdUpstream.GetMetadata()
				Expect(virtualService).To(Equal(createdUpstream))
			})
		})
		Describe("Get", func() {
			It("gets a crd from the name", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				vService := NewTestVirtualService("something", NewTestRoute1())
				_, err = client.V1().VirtualServices().Create(vService)
				Expect(err).NotTo(HaveOccurred())
				created, err := client.V1().VirtualServices().Get(vService.Name)
				Expect(err).NotTo(HaveOccurred())
				vService.Metadata = created.Metadata
				Expect(created).To(Equal(vService))
			})
		})
		Describe("Update", func() {
			It("updates a crd from the item", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				vService := NewTestVirtualService("something", NewTestRoute1())
				created, err := client.V1().VirtualServices().Create(vService)
				Expect(err).NotTo(HaveOccurred())
				// need to set resource ver
				vService.Metadata = created.GetMetadata()
				vService.Metadata.Annotations["just_for_this_test"] = "bar"
				updated, err := client.V1().VirtualServices().Update(vService)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated.Metadata.Annotations).To(HaveKey("just_for_this_test"))
				vService.Metadata = updated.GetMetadata()
				Expect(updated).To(Equal(vService))
			})
		})
		Describe("Delete", func() {
			It("deletes a crd from the name", func() {
				cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
				Expect(err).NotTo(HaveOccurred())
				client, err := NewStorage(cfg, namespace, syncFreq)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().Register()
				Expect(err).NotTo(HaveOccurred())
				vService := NewTestVirtualService("something", NewTestRoute1())
				_, err = client.V1().VirtualServices().Create(vService)
				Expect(err).NotTo(HaveOccurred())
				err = client.V1().VirtualServices().Delete(vService.Name)
				Expect(err).NotTo(HaveOccurred())
				_, err = client.V1().VirtualServices().Get(vService.Name)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
