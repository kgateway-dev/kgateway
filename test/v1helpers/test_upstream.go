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
	"net/url"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/test/helpers"
	testgrpcservice "github.com/solo-io/gloo/test/v1helpers/test_grpc_service"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type ReceivedRequest struct {
	Method      string
	URL         *url.URL
	Body        []byte
	Host        string
	GRPCRequest proto.Message
	Port        uint32
}

func NewTestHttpUpstream(ctx context.Context, addr string) *TestUpstream {
	backendPort, responses := runTestServer(ctx, "", false)
	return newTestUpstream(addr, []uint32{backendPort}, responses)
}

func NewTestHttpUpstreamWithReply(ctx context.Context, addr, reply string) *TestUpstream {
	backendPort, responses := runTestServer(ctx, reply, false)
	return newTestUpstream(addr, []uint32{backendPort}, responses)
}

func NewTestHttpUpstreamWithReplyAndHealthReply(ctx context.Context, addr, reply, healthReply string) *TestUpstream {
	backendPort, responses := runTestServerWithHealthReply(ctx, reply, healthReply, false)
	return newTestUpstream(addr, []uint32{backendPort}, responses)
}

func NewTestHttpsUpstreamWithReply(ctx context.Context, addr, reply string) *TestUpstream {
	backendPort, responses := runTestServer(ctx, reply, true)
	return newTestUpstream(addr, []uint32{backendPort}, responses)
}

func NewTestGRPCUpstream(ctx context.Context, addr string, replicas int) *TestUpstream {
	grpcServices := make([]*testgrpcservice.TestGRPCServer, replicas)
	for i := range grpcServices {
		grpcServices[i] = testgrpcservice.RunServer(ctx)
	}
	received := make(chan *ReceivedRequest, 100)
	for _, srv := range grpcServices {
		srv := srv
		go func() {
			defer GinkgoRecover()
			for r := range srv.C {
				received <- &ReceivedRequest{GRPCRequest: r, Port: srv.Port}
			}
		}()
	}
	ports := make([]uint32, 0, len(grpcServices))
	for _, v := range grpcServices {
		ports = append(ports, v.Port)
	}

	us := newTestUpstream(addr, ports, received)
	us.Upstream.UseHttp2 = &wrappers.BoolValue{Value: true}
	us.GrpcServers = grpcServices
	return us
}

type TestUpstream struct {
	Upstream    *gloov1.Upstream
	C           <-chan *ReceivedRequest
	Address     string
	Port        uint32
	GrpcServers []*testgrpcservice.TestGRPCServer
}

func (tu *TestUpstream) FailGrpcHealthCheck() *testgrpcservice.TestGRPCServer {
	for _, v := range tu.GrpcServers[:len(tu.GrpcServers)-1] {
		v.HealthChecker.Fail()
	}
	return tu.GrpcServers[len(tu.GrpcServers)-1]
}

var id = 0

func newTestUpstream(addr string, ports []uint32, responses <-chan *ReceivedRequest) *TestUpstream {
	id += 1
	hosts := make([]*static_plugin_gloo.Host, len(ports))
	for i, port := range ports {
		hosts[i] = &static_plugin_gloo.Host{
			Addr: addr,
			Port: port,
		}
	}
	u := &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      fmt.Sprintf("local-%d", id),
			Namespace: "default",
		},
		UpstreamType: &gloov1.Upstream_Static{
			Static: &static_plugin_gloo.UpstreamSpec{
				Hosts: hosts,
			},
		},
	}

	return &TestUpstream{
		Upstream: u,
		C:        responses,
		Port:     ports[0],
	}
}

func runTestServer(ctx context.Context, reply string, serveTls bool) (uint32, <-chan *ReceivedRequest) {
	return runTestServerWithHealthReply(ctx, reply, "OK", serveTls)
}

func runTestServerWithHealthReply(ctx context.Context, reply, healthReply string, serveTls bool) (uint32, <-chan *ReceivedRequest) {
	bodyChan := make(chan *ReceivedRequest, 100)
	handlerFunc := func(rw http.ResponseWriter, r *http.Request) {
		var rr ReceivedRequest
		rr.Method = r.Method
		if reply != "" {
			_, _ = rw.Write([]byte(reply))
		} else if r.Body != nil {
			body, _ := ioutil.ReadAll(r.Body)
			_ = r.Body.Close()
			if len(body) != 0 {
				rr.Body = body
				_, _ = rw.Write(body)
			}
		}

		rr.Host = r.Host
		rr.URL = r.URL

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

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(handlerFunc))
	mux.Handle("/health", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte(healthReply))
	}))

	go func() {
		defer GinkgoRecover()
		h := &http.Server{Handler: mux}
		if serveTls {
			certs, err := tls.X509KeyPair([]byte(helpers.Certificate()), []byte(helpers.PrivateKey()))
			if err != nil {
				Expect(err).NotTo(HaveOccurred())
			}
			listener = tls.NewListener(listener, &tls.Config{
				Certificates: []tls.Certificate{certs},
			})
		}

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
	TestUpstreamReachableWithOffset(2, envoyPort, tu, rootca)
}

func TestUpstreamReachableWithOffset(offset int, envoyPort uint32, tu *TestUpstream, rootca *string) {
	body := []byte("solo.io test")

	ExpectHttpOK(body, rootca, envoyPort, "")

	timeout := time.After(15 * time.Second)
	var receivedRequest *ReceivedRequest
	for {
		select {
		case <-timeout:
			if receivedRequest != nil {
				fmt.Fprintf(GinkgoWriter, "last received request: %v", *receivedRequest)
			}
			Fail("timeout testing upstream reachability")
		case receivedRequest = <-tu.C:
			if receivedRequest.Method == "POST" &&
				bytes.Equal(receivedRequest.Body, body) {
				return
			}
		}
	}

}

func ExpectHttpOK(body []byte, rootca *string, envoyPort uint32, response string) {
	ExpectHttpOKWithOffset(1, body, rootca, envoyPort, response)
}

func ExpectHttpOKWithOffset(offset int, body []byte, rootca *string, envoyPort uint32, response string) {
	ExpectHttpStatusWithOffset(offset+1, body, rootca, envoyPort, response, http.StatusOK)
}

func ExpectHttpUnavailableWithOffset(offset int, body []byte, rootca *string, envoyPort uint32, response string) {
	ExpectHttpStatusWithOffset(offset+1, body, rootca, envoyPort, response, http.StatusServiceUnavailable)
}

func ExpectHttpStatusWithOffset(offset int, body []byte, rootca *string, envoyPort uint32, response string, status int) {

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
		if res.StatusCode != status {
			return fmt.Errorf("received status code (%v) is not expected status code (%v)", res.StatusCode, status)
		}

		return nil
	}, "30s", "1s").Should(BeNil())

	if response != "" {
		body, err := ioutil.ReadAll(res.Body)
		ExpectWithOffset(offset, err).NotTo(HaveOccurred())
		defer res.Body.Close()
		ExpectWithOffset(offset, string(body)).To(Equal(response))
	}
}

func ExpectGrpcHealthOK(rootca *string, envoyPort uint32, service string) {
	EventuallyWithOffset(2, func() error {
		// send a request with a body

		opts := []grpc.DialOption{grpc.WithBlock()}
		if rootca != nil {
			caCertPool := x509.NewCertPool()
			ok := caCertPool.AppendCertsFromPEM([]byte(*rootca))
			if !ok {
				Fail("ca cert is not OK")
			}
			creds := credentials.NewTLS(&tls.Config{
				ClientCAs:          caCertPool,
				InsecureSkipVerify: true,
			})

			opts = append(opts, grpc.WithTransportCredentials(creds))
		} else {
			opts = append(opts, grpc.WithInsecure())
		}
		conn, err := grpc.Dial(fmt.Sprintf("%s:%d", "localhost", envoyPort), opts...)
		ExpectWithOffset(2, err).NotTo(HaveOccurred())
		defer conn.Close()

		c := healthpb.NewHealthClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		resp, err := c.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
		cancel()
		if err != nil {
			return err
		}
		if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			return fmt.Errorf("%v is not SERVING", resp.GetStatus())
		}
		return nil
	}, "30s", "1s").Should(BeNil())
}
