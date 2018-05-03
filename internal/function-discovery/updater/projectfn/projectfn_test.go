package projectfn_test

import (
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/client/apps"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/client/routes"
	"github.com/solo-io/gloo/internal/function-discovery/updater/projectfn/imported/models"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/internal/function-discovery/updater/projectfn"
)

type mockTransport struct {
	appResp   apps.GetAppsOK
	appErr    error
	appCursor map[string]apps.GetAppsOK

	routesResp   routes.GetAppsAppRoutesOK
	routesErr    error
	routesCursor map[string]routes.GetAppsAppRoutesOK
}

func (m *mockTransport) Submit(c *runtime.ClientOperation) (interface{}, error) {
	switch c.ID {
	case "GetApps":
		params := c.Params.(*apps.GetAppsParams)
		if params.Cursor == nil || *params.Cursor == "" {
			return &m.appResp, m.appErr
		} else {
			res := m.appCursor[*params.Cursor]
			return &res, nil
		}
	case "GetAppsAppRoutes":
		params := c.Params.(*routes.GetAppsAppRoutesParams)
		if params.Cursor == nil || *params.Cursor == "" {
			return &m.routesResp, m.routesErr
		} else {
			res := m.routesCursor[*params.Cursor]
			return &res, nil
		}
	}
	fmt.Printf("%v\n", *c)
	panic("unknown op")
}

var _ = Describe("Projectfn", func() {

	var transport *mockTransport

	BeforeEach(func() {
		transport = &mockTransport{
			appCursor:    make(map[string]apps.GetAppsOK),
			routesCursor: make(map[string]routes.GetAppsAppRoutesOK),
		}
	})

	Context("GetFuncs", func() {

		It("should detect function", func() {
			transport.appResp = apps.GetAppsOK{
				Payload: &models.AppsWrapper{
					Apps: models.AppsWrapperApps{{
						Name: "app",
					}},
				},
			}
			transport.routesResp = routes.GetAppsAppRoutesOK{
				Payload: &models.RoutesWrapper{
					Routes: models.RoutesWrapperRoutes{{
						Path: "/func",
					}},
				},
			}

			var mockResolver *helpers.MockResolver
			mockResolver = &helpers.MockResolver{Result: "somehost"}

			us := &v1.Upstream{
				Name:     "default-my-release-fn-api-80",
				Type:     kubernetes.UpstreamTypeKube,
				Metadata: &v1.Metadata{Namespace: "gloo-system"},
				Spec: kubernetes.EncodeUpstreamSpec(kubernetes.UpstreamSpec{
					ServiceName:      "my-release-fn-api",
					ServiceNamespace: "default",
					ServicePort:      80,
				}),
			}

			funcs, err := GetFuncsWithTransport(mockResolver, us, transport)
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(1))
			t, err := rest.DecodeFunctionSpec(funcs[0].Spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(t.Path).To(Equal("/r/app/func"))

		})
	})

	Context("NewFnRetreiver", func() {
		var fnRetreiver *FnRetreiver

		BeforeEach(func() {

			fr, err := NewFnRetreiver("http://blah:90/")
			Expect(err).NotTo(HaveOccurred())
			fr.SetTransport(transport)
			fnRetreiver = fr
			transport.appResp = apps.GetAppsOK{
				Payload: &models.AppsWrapper{},
			}
			transport.routesResp = routes.GetAppsAppRoutesOK{
				Payload: &models.RoutesWrapper{},
			}
		})

		It("should detect function", func() {
			transport.appResp = apps.GetAppsOK{
				Payload: &models.AppsWrapper{
					Apps: models.AppsWrapperApps{{
						Name: "app",
					}},
				},
			}
			transport.routesResp = routes.GetAppsAppRoutesOK{
				Payload: &models.RoutesWrapper{
					Routes: models.RoutesWrapperRoutes{{
						Path: "/func",
					}},
				},
			}

			funcs, err := fnRetreiver.GetFuncs()
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(1))
			Expect(funcs[0].Appname).To(ContainSubstring("app"))
			Expect(funcs[0].Route).To(Equal("/r/app/func"))

		})

		It("should not detect function when there are no apps", func() {

			funcs, err := fnRetreiver.GetFuncs()
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(0))

		})
		It("should not detect function when there is apps with no routes", func() {

			transport.appResp = apps.GetAppsOK{
				Payload: &models.AppsWrapper{
					Apps: models.AppsWrapperApps{{
						Name: "app",
					}},
				},
			}
			funcs, err := fnRetreiver.GetFuncs()
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(0))

		})

		It("should gete all apps when paginated", func() {
			transport.appResp = apps.GetAppsOK{
				Payload: &models.AppsWrapper{
					Apps: models.AppsWrapperApps{{
						Name: "app",
					}},
					NextCursor: "zbam",
				},
			}
			transport.appCursor["zbam"] = apps.GetAppsOK{
				Payload: &models.AppsWrapper{
					Apps: models.AppsWrapperApps{{
						Name: "app2",
					}},
				},
			}
			transport.routesResp = routes.GetAppsAppRoutesOK{
				Payload: &models.RoutesWrapper{
					Routes: models.RoutesWrapperRoutes{{
						Path: "/func",
					}},
					NextCursor: "zbam2",
				},
			}

			transport.routesCursor["zbam2"] = routes.GetAppsAppRoutesOK{
				Payload: &models.RoutesWrapper{
					Routes: models.RoutesWrapperRoutes{{
						Path: "/func2",
					}},
				},
			}

			funcs, err := fnRetreiver.GetFuncs()
			Expect(err).NotTo(HaveOccurred())
			Expect(funcs).To(HaveLen(4))
			for _, f := range []string{"func", "func2"} {
				for _, app := range []string{"app", "app2"} {
					expected := Function{
						Appname:  app,
						Funcname: f,
						Route:    "/r/" + app + "/" + f,
					}
					Expect(funcs).To(ContainElement(expected))
				}
			}
		})

	})
})
