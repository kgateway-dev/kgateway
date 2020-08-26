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
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

// TODO (Shane): remove
// Intentionally focused describe to test CI/CD version
var _ = FDescribe("Zipkin", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		envoyInstance  *services.EnvoyInstance
		tu             *v1helpers.TestUpstream
		writeNamespace string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		writeNamespace = defaults.GlooSystem
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: false,
				DisableUds:     true,
				DisableFds:     true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		_, err = testClients.GatewayClient.Write(getGrpcJsonGateway(), clients.WriteOpts{Ctx: ctx})
		Expect(err).ToNot(HaveOccurred())

		err = envoyInstance.RunWithConfig(8080, "./envoyconfigs/zipkin-envoy-conf.yaml")
		Expect(err).NotTo(HaveOccurred())

		tu = v1helpers.NewTestGRPCUpstream(ctx, envoyInstance.LocalAddr(), 1)
		_, err = testClients.UpstreamClient.Write(tu.Upstream, clients.WriteOpts{})
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
			req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/", "localhost", defaults.HttpPort), nil)
			Expect(err).NotTo(HaveOccurred())
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

		Eventually(testRequest, 10, 1).Should(ContainSubstring(`<title>Envoy Admin</title>`))

		timeout := time.After(5 * time.Second)
		select {
		case <-timeout:
			Fail("timeout waiting for zipkin api to be hit")
		case <-apiHit:
			// Successfully received a trace req, do nothing
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	})
})
