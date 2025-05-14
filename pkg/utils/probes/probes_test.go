package probes_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/pkg/utils/probes"
)

var _ = Describe("Probes", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	It("starts a default server", func() {
		StartLivenessProbeServer(ctx)

		Eventually(func(g Gomega) {
			resp, err := http.Get("http://127.0.0.1:8765/healthz")
			g.Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

			b, err := io.ReadAll(resp.Body)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(b)).To(Equal("OK\n"))
		}).WithTimeout(time.Second).WithPolling(time.Millisecond * 100).Should(Succeed())
	})

	It("starts a custom server", func() {
		params := ServerParams{
			Port:         9876,
			Path:         "foobar",
			ResponseCode: http.StatusTeapot,
			ResponseBody: "scoobydoo",
		}
		StartServer(ctx, params)

		Eventually(func(g Gomega) {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/%s", params.Port, params.Path))
			g.Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			g.Expect(resp.StatusCode).To(Equal(params.ResponseCode))

			b, err := io.ReadAll(resp.Body)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(b)).To(Equal(params.ResponseBody))
		}).WithTimeout(time.Second).WithPolling(time.Millisecond * 100).Should(Succeed())
	})

	It("doesn't respond after server cancelled", func() {
		StartLivenessProbeServer(ctx)

		Eventually(func(g Gomega) {
			resp, err := http.Get("http://127.0.0.1:8765/healthz")
			g.Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

			b, err := io.ReadAll(resp.Body)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(string(b)).To(Equal("OK\n"))
		}).WithTimeout(time.Second).WithPolling(time.Millisecond * 100).Should(Succeed())

		cancel()

		Eventually(func(g Gomega) {
			resp, err := http.Get("http://127.0.0.1:8765/healthz")
			if err == nil {
				resp.Body.Close()
			}
			g.Expect(err).To(HaveOccurred())
		}).WithTimeout(time.Second * 2).WithPolling(time.Millisecond * 100).Should(Succeed())

	})
})
