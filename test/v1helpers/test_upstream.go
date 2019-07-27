package v1helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/gogo/protobuf/proto"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	testgrpcservice "github.com/solo-io/gloo/test/v1helpers/test_grpc_service"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type ReceivedRequest struct {
	Method      string
	Body        []byte
	Host        string
	GRPCRequest proto.Message
}

func NewTestHttpUpstream(ctx context.Context, addr string) *TestUpstream {
	backendPort, responses := runTestServer(ctx)
	return newTestUpstream(addr, backendPort, responses)
}

func NewTestGRPCUpstream(ctx context.Context, addr string) *TestUpstream {
	srv := testgrpcservice.RunServer(ctx)
	received := make(chan *ReceivedRequest, 100)
	go func() {
		defer GinkgoRecover()
		for r := range srv.C {
			received <- &ReceivedRequest{GRPCRequest: r}
		}
	}()

	us := newTestUpstream(addr, srv.Port, received)
	return us
}

type TestUpstream struct {
	Upstream *gloov1.Upstream
	C        <-chan *ReceivedRequest
	Address  string
	Port     uint32
}

var id = 0

func newTestUpstream(addr string, port uint32, responses <-chan *ReceivedRequest) *TestUpstream {
	id += 1
	u := &gloov1.Upstream{
		Metadata: core.Metadata{
			Name:      fmt.Sprintf("local-%d", id),
			Namespace: "default",
		},
		UpstreamSpec: &gloov1.UpstreamSpec{
			UpstreamType: &gloov1.UpstreamSpec_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{{
						Addr: addr,
						Port: port,
					}},
				},
			},
		},
	}

	return &TestUpstream{
		Upstream: u,
		C:        responses,
		Address:  fmt.Sprintf("%s:%d", addr, port),
		Port:     port,
	}
}

func runTestServer(ctx context.Context) (uint32, <-chan *ReceivedRequest) {
	bodyChan := make(chan *ReceivedRequest, 100)
	handlerFunc := func(rw http.ResponseWriter, r *http.Request) {
		var rr ReceivedRequest
		rr.Method = r.Method
		if r.Body != nil {
			body, _ := ioutil.ReadAll(r.Body)
			_ = r.Body.Close()
			if len(body) != 0 {
				rr.Body = body
				_, _ = rw.Write(body)
			}
		}

		rr.Host = r.Host

		bodyChan <- &rr
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(err)
	}

	handler := http.HandlerFunc(handlerFunc)
	go func() {
		defer GinkgoRecover()
		h := &http.Server{Handler: handler}
		go func() {
			defer GinkgoRecover()
			if err := h.Serve(listener); err != nil {
				if err != http.ErrServerClosed {
					panic(err)
				}
			}
		}()

		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = h.Shutdown(ctx)
		cancel()
		// close channel, the http handler may panic but this should be caught by the http code.
		close(bodyChan)
	}()
	return uint32(port), bodyChan
}

func TestUpstreamReachable(envoyPort uint32, tu *TestUpstream, rootca *string) {
	body := []byte("solo.io test")

	ExpectHttpOK(body, rootca, envoyPort, "")

	EventuallyWithOffset(1, tu.C, "5s", "0.2s").Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
		"Method": Equal("POST"),
		"Body":   Equal(body),
	}))))
}

func ExpectHttpOK(body []byte, rootca *string, envoyPort uint32, response string) {

	var res *http.Response
	EventuallyWithOffset(2, func() error {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)

		var client http.Client

		scheme := "http"
		if rootca != nil {
			scheme = "https"
			caCertPool := x509.NewCertPool()
			ok := caCertPool.AppendCertsFromPEM([]byte(*rootca))
			if !ok {
				return fmt.Errorf("ca cert is not OK")
			}

			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:            caCertPool,
					InsecureSkipVerify: true,
				},
			}
		}

		var err error
		res, err = client.Post(fmt.Sprintf("%s://%s:%d/1", scheme, "localhost", envoyPort), "application/octet-stream", &buf)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%v is not OK", res.StatusCode)
		}

		return nil
	}, "10s", ".5s").Should(BeNil())

	if response != "" {
		body, err := ioutil.ReadAll(res.Body)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		defer res.Body.Close()
		ExpectWithOffset(2, string(body)).To(Equal(response))
	}
}