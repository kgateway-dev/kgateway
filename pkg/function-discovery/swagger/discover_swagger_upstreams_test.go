package swagger_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery"
	. "github.com/solo-io/gloo/pkg/function-discovery/swagger"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/swagger"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/plugins/rest"
)

var _ = Describe("DiscoverSwaggerUpstreams", func() {
	Describe("happy path", func() {
		Context("upstream for a service that serves a swagger spec", func() {
			// create a test swagger server
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/swagger" {
					log.Printf("hit")
					fmt.Fprint(w, swaggerDoc)
					return
				}
				http.NotFound(w, r)
			}))
			BeforeEach(func() {
				//srv.Start()
			})
			AfterEach(func() {
				srv.Close()
			})
			It("returns annotations with swagger doc url and service info for REST", func() {
				addr := strings.TrimPrefix(srv.URL, "http://")
				d := NewSwaggerDetector(nil)
				svc, annotations, err := d.DetectFunctionalService(&v1.Upstream{Name: "Test"}, addr)
				Expect(err).To(BeNil())
				Expect(annotations).To(HaveKeyWithValue(swagger.AnnotationKeySwaggerURL, "http://"+addr+"/v1/swagger"))
				Expect(annotations).To(HaveKey(functiondiscovery.DiscoveryTypeAnnotationKey))

				Expect(svc).To(Equal(&v1.ServiceInfo{Type: rest.ServiceTypeREST}))
			})
		})
	})
})

const swaggerDoc = `{
  "swagger": "2.0",
  "info": {
    "version": "1.0.0",
    "title": "Swagger Petstore",
    "description": "A sample API that uses a petstore as an example to demonstrate features in the swagger-2.0 specification",
    "termsOfService": "http://swagger.io/terms/",
    "contact": {
      "name": "Swagger API Team"
    },
    "license": {
      "name": "MIT"
    }
  },
  "host": "petstore.swagger.io",
  "basePath": "/api",
  "schemes": [
    "http"
  ],
  "consumes": [
    "application/json"
  ],
  "produces": [
    "application/json"
  ],
  "paths": {
    "/pets": {
      "get": {
        "description": "Returns all pets from the system that the user has access to",
        "produces": [
          "application/json"
        ],
        "responses": {
          "200": {
            "description": "A list of pets.",
            "schema": {
              "type": "array",
              "items": {
                "$ref": "#/definitions/Pet"
              }
            }
          }
        }
      }
    }
  },
  "definitions": {
    "Pet": {
      "type": "object",
      "required": [
        "id",
        "name"
      ],
      "properties": {
        "id": {
          "type": "integer",
          "format": "int64"
        },
        "name": {
          "type": "string"
        },
        "tag": {
          "type": "string"
        }
      }
    }
  }
}`
