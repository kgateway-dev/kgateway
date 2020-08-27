package e2e_test

import (
	"context"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var _ = Describe("Zipkin config loading", func() {
	var (
		cancel        context.CancelFunc
		envoyInstance *services.EnvoyInstance
	)

	BeforeEach(func() {
		_, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

	basicReq := func() func() (string, error) {
		return func() (string, error) {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", 11082), nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("Content-Type", "application/json")

			// Set a random trace ID
			req.Header.Set("x-client-trace-id", "test-trace-id-1234567890")

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			return string(body), err
		}
	}

	It("should send trace msgs to the zipkin server", func() {
		err := envoyInstance.RunWithConfig(int(defaults.HttpPort), "./envoyconfigs/zipkin-envoy-conf.yaml")
		Expect(err).NotTo(HaveOccurred())

		apiHit := make(chan bool, 1)

		// Start a dummy server listening on 9411 for Zipkin requests
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/api/v2/spans")) // Zipkin json collector API
			fmt.Fprintf(w, "Dummy Zipkin Collector received request on - %q", html.EscapeString(r.URL.Path))
			apiHit <- true
		})

		server := &http.Server{Addr: ":9411", Handler: nil}
		go func() {
			server.ListenAndServe()
		}()

		testRequest := basicReq()

		Eventually(testRequest, 15, 1).Should(ContainSubstring(`<title>Envoy Admin</title>`))

		truez := true
		Eventually(apiHit, 5*time.Second).Should(Receive(&truez))

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	})

	It("should fail to load bad config", func() {
		err := envoyInstance.RunWithConfig(int(defaults.HttpPort), "./envoyconfigs/zipkin-envoy-invalid-conf.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(And(ContainSubstring("can't unmarshal"), ContainSubstring(`unknown field "invalid_field"`))))
	})
})
