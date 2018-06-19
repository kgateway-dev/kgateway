package swagger_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	. "github.com/solo-io/gloo/internal/function-discovery/updater/swagger"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/plugins/rest"
)

var _ = Describe("GetSwaggerFuncs", func() {

	// create a test swagger server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, swaggerDoc)
	}))
	BeforeEach(func() {
		//srv.Start()
	})
	AfterEach(func() {
		srv.Close()
	})
	It("returns funcs for the upstream", func() {
		u := strings.Split(srv.URL, ":")
		addr := strings.TrimPrefix(u[1], "//")
		port, _ := strconv.Atoi(u[2])

		us := &v1.Upstream{
			Name: "something",
			Type: static.UpstreamTypeService,
			Metadata: &v1.Metadata{Annotations: map[string]string{
				AnnotationKeySwaggerURL: srv.URL + "/anything",
			}},
			Spec: static.EncodeUpstreamSpec(static.UpstreamSpec{
				Hosts: []static.Host{
					{
						Addr: addr,
						Port: uint32(port),
					},
				},
			}),
		}
		funcs, err := GetFuncs(us)
		Expect(err).NotTo(HaveOccurred())
		Expect(funcs).To(HaveLen(1))
		str := ""
		expectedFn := &v1.Function{
			Name: "get.pets",
			Spec: rest.EncodeFunctionSpec(rest.Template{
				Path:   "/api/pets",
				Header: map[string]string{":method": "GET"},
				Body:   &str,
			}),
		}
		Expect(funcs[0]).To(Equal(expectedFn))
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
