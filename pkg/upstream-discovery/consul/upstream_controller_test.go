package consul

import (
	"os"

	"time"

	"sort"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	consulstorage "github.com/solo-io/gloo/pkg/storage/consul"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

var _ = Describe("Consul UpstreamController Units", func() {
	Describe("uniqueTagSets", func() {
		It("takes a list of unsorted, duplicated tags and sorts and de-dupes them", func() {
			input := [][]string{
				{"foo", "bar", "baz"},
				{"foo", "baz"},
				{"baz", "foo"},
				{"baz", "fooa"},
			}
			out := uniqueTagSets(input)
			Expect(out).To(Equal([][]string{
				{"baz", "foo"},
				{"baz", "fooa"},
				{"bar", "baz", "foo"},
			}))
		})
	})
})

var _ = Describe("Consul UpstreamController Integration", func() {
	if os.Getenv("RUN_CONSUL_TESTS") != "1" {
		log.Printf("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}

	var (
		consulFactory  *localhelpers.ConsulFactory
		consulInstance *localhelpers.ConsulInstance
		err            error
	)

	var _ = BeforeSuite(func() {
		consulFactory, err = localhelpers.NewConsulFactory()
		helpers.Must(err)
		consulInstance, err = consulFactory.NewConsulInstance()
		helpers.Must(err)
		err = consulInstance.Run()
		helpers.Must(err)
	})

	var _ = AfterSuite(func() {
		consulInstance.Clean()
		consulFactory.Clean()
	})

	var (
		consul   *api.Client
		cfg      = api.DefaultConfig()
		rootPath string
	)

	BeforeEach(func() {
		var err error
		consul, err = api.NewClient(cfg)
		helpers.Must(err)
		rootPath = helpers.RandString(4)
	})
	AfterEach(func() {
		consul.KV().DeleteTree(rootPath, nil)
	})

	Describe("uniqueTagSets", func() {
		It("takes a list of unsorted, duplicated tags and sorts and de-dupes them", func() {
			cfg.WaitTime = time.Minute

			store, err := consulstorage.NewStorage(cfg, rootPath, time.Minute)
			Expect(err).NotTo(HaveOccurred())

			ctl, err := NewUpstreamController(cfg, store)
			Expect(err).NotTo(HaveOccurred())

			ch := make(chan struct{})
			defer close(ch)
			go ctl.Run(ch)

			consul, err := api.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			svc2 := newConsulSvc("svc2", nil, "3.4.5.6", 3456)
			svc3notags := newConsulSvc("svc3", []string{}, "4.5.6.7", 3456)
			svc3sometags := newConsulSvc("svc3", []string{"a"}, "5.6.7.8", 3456)
			svc3alltags := newConsulSvc("svc3", []string{"a", "b"}, "6.7.8.9", 3456)
			svc3alltags2 := newConsulSvc("svc3", []string{"a", "b"}, "7.7.8.9", 3456)

			err = consul.Agent().ServiceRegister(svc2)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3notags)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3sometags)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3alltags)
			Expect(err).NotTo(HaveOccurred())
			err = consul.Agent().ServiceRegister(svc3alltags2)
			Expect(err).NotTo(HaveOccurred())

			var ups []*v1.Upstream

			Eventually(func() ([]*v1.Upstream, error) {
				select {
				case err := <-ctl.Error():
					return nil, err
				default:
					us, err := store.V1().Upstreams().List()
					removeResourceVersion(us)
					sort.SliceStable(us, func(i, j int) bool {
						return us[i].Name < us[j].Name
					})
					for i, u := range us {
						if u.Name == "consul" {
							us = append(us[:i], us[i+1:]...)
							break
						}
					}
					ups = us
					return us, err
				}
			}).Should(HaveLen(4))

			Expect(ups).To(ContainElement(&v1.Upstream{
				Name: "svc2",
				Type: "consul",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"service_name": {
							Kind: &types.Value_StringValue{
								StringValue: "svc2",
							},
						},
						"service_tags": {
							Kind: &types.Value_ListValue{
								ListValue: &types.ListValue{
									Values: nil,
								},
							},
						},
					},
				},
				Metadata: &v1.Metadata{
					Annotations: map[string]string{
						"generated_by": "consul-upstream-discovery",
					},
				},
			}))

			Expect(ups).To(ContainElement(&v1.Upstream{
				Name: "svc2",
				Type: "consul",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"service_name": {
							Kind: &types.Value_StringValue{
								StringValue: "svc2",
							},
						},
						"service_tags": {
							Kind: &types.Value_ListValue{
								ListValue: &types.ListValue{
									Values: nil,
								},
							},
						},
					},
				},
				Metadata: &v1.Metadata{
					Annotations: map[string]string{
						"generated_by": "consul-upstream-discovery",
					},
				},
			}))

			Expect(ups).To(ContainElement(&v1.Upstream{
				Name: "svc3",
				Type: "consul",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"service_name": {
							Kind: &types.Value_StringValue{
								StringValue: "svc3",
							},
						},
						"service_tags": {
							Kind: &types.Value_ListValue{
								ListValue: &types.ListValue{
									Values: nil,
								},
							},
						},
					},
				},
				Metadata: &v1.Metadata{
					Annotations: map[string]string{
						"generated_by": "consul-upstream-discovery",
					},
				},
			}))

			Expect(ups).To(ContainElement(&v1.Upstream{
				Name: "svc3-a",
				Type: "consul",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"service_name": {
							Kind: &types.Value_StringValue{
								StringValue: "svc3",
							},
						},
						"service_tags": {
							Kind: &types.Value_ListValue{
								ListValue: &types.ListValue{
									Values: []*types.Value{
										{
											Kind: &types.Value_StringValue{
												StringValue: "a",
											},
										},
									},
								},
							},
						},
					},
				},
				Metadata: &v1.Metadata{
					Annotations: map[string]string{
						"generated_by": "consul-upstream-discovery",
					},
				},
			}))

			Expect(ups).To(ContainElement(&v1.Upstream{
				Name: "svc3-a-b",
				Type: "consul",
				Spec: &types.Struct{
					Fields: map[string]*types.Value{
						"service_name": {
							Kind: &types.Value_StringValue{
								StringValue: "svc3",
							},
						},
						"service_tags": {
							Kind: &types.Value_ListValue{
								ListValue: &types.ListValue{
									Values: []*types.Value{
										{
											Kind: &types.Value_StringValue{
												StringValue: "a",
											},
										},
										{
											Kind: &types.Value_StringValue{
												StringValue: "b",
											},
										},
									},
								},
							},
						},
					},
				},
				Metadata: &v1.Metadata{
					Annotations: map[string]string{
						"generated_by": "consul-upstream-discovery",
					},
				},
			}))
		})
	})
})

func newConsulSvc(name string, tags []string, address string, port int) *api.AgentServiceRegistration {
	return &api.AgentServiceRegistration{
		ID:      helpers.RandString(4),
		Name:    name,
		Tags:    tags,
		Port:    port,
		Address: address,
	}
}

func removeResourceVersion(items []*v1.Upstream) {
	for _, item := range items {
		item.Metadata.ResourceVersion = ""
	}
}
