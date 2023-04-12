package v1helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
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

// TestUpstream is a testing utility (used in in-memory e2e tests) to compose the following concepts:
//  1. Running an application with a custom response message (see: runTestServer)
//  2. Configuring an Upstream object to route to that application (see: newTestUpstream)
//  3. Utility methods for asserting that traffic was successfully routed to the application (see: Assertion Utilities)
type TestUpstream struct {
	Upstream    *gloov1.Upstream
	C           <-chan *ReceivedRequest
	CResp       <-chan *ReturnedResponse
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

type ReceivedRequest struct {
	Method      string
	Headers     map[string][]string
	URL         *url.URL
	Body        []byte
	Host        string
	GRPCRequest proto.Message
	Port        uint32
}

func (rr *ReceivedRequest) String() string {
	var grpcRequest string
	if rr.GRPCRequest != nil {
		grpcRequest = rr.GRPCRequest.String()
	}
	return fmt.Sprintf(`Method: %s
Headers: %v
URL: %s
Body: %s
Host: %s
GRPCRequest: %s
Port: %d`, rr.Method, rr.Headers, rr.URL.String(), string(rr.Body), rr.Host,
		grpcRequest, rr.Port)
}

type ReturnedResponse struct {
	Code    int
	Body    []byte
	Headers map[string][]string
}

func (rr *ReturnedResponse) String() string {
	return fmt.Sprintf(`Code: %d
Body: %s
Headers: %v`, rr.Code, string(rr.Body), rr.Headers)
}

const (
	NO_TLS = iota
	TLS
	MTLS
)

type UpstreamTlsRequired int

// Test Upstream Factory Utilities
//
// Below are a collection of methods that can be used to create a TestUpstream with a certain behavior

func NewTestHttpUpstream(ctx context.Context, addr string) *TestUpstream {
	backendPort, requests, responses := runTestServer(ctx, "", NO_TLS)
	return newTestUpstream(addr, []uint32{backendPort}, requests, responses)
}

func NewTestHttpUpstreamWithTls(ctx context.Context, addr string, tlsServer UpstreamTlsRequired) *TestUpstream {
	backendPort, requests, responses := runTestServer(ctx, "", tlsServer)
	return newTestUpstream(addr, []uint32{backendPort}, requests, responses)
}

func NewTestHttpUpstreamWithReply(ctx context.Context, addr, reply string) *TestUpstream {
	backendPort, requests, responses := runTestServer(ctx, reply, NO_TLS)
	return newTestUpstream(addr, []uint32{backendPort}, requests, responses)
}

func NewTestHttpUpstreamWithReplyAndHealthReply(ctx context.Context, addr, reply, healthReply string) *TestUpstream {
	backendPort, requests, responses := runTestServerWithHealthReply(ctx, reply, healthReply, NO_TLS)
	return newTestUpstream(addr, []uint32{backendPort}, requests, responses)
}

func NewTestHttpsUpstreamWithReply(ctx context.Context, addr, reply string) *TestUpstream {
	backendPort, requests, responses := runTestServer(ctx, reply, TLS)
	return newTestUpstream(addr, []uint32{backendPort}, requests, responses)
}

func NewTestGRPCUpstream(ctx context.Context, addr string, replicas int) *TestUpstream {
	grpcServices := make([]*testgrpcservice.TestGRPCServer, replicas)
	for i := range grpcServices {
		grpcServices[i] = testgrpcservice.RunServer(ctx)
	}
	received := make(chan *ReceivedRequest, 100)
	returned := make(chan *ReturnedResponse, 100)
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

	us := newTestUpstream(addr, ports, received, returned)
	us.Upstream.UseHttp2 = &wrappers.BoolValue{Value: true}
	us.GrpcServers = grpcServices
	return us
}

var testUpstreamId = 0

// newTestUpstream creates a static Upstream that can route traffic to a set of ports for a given address
// It contains a unique name (since tests may run in parallel), with a suffix id that increases each invocation
func newTestUpstream(addr string, ports []uint32, requests <-chan *ReceivedRequest, responses <-chan *ReturnedResponse) *TestUpstream {
	testUpstreamId += 1
	hosts := make([]*static_plugin_gloo.Host, len(ports))
	for i, port := range ports {
		hosts[i] = &static_plugin_gloo.Host{
			Addr: addr,
			Port: port,
		}
	}
	u := &gloov1.Upstream{
		Metadata: &core.Metadata{
			Name:      fmt.Sprintf("local-test-upstream-%d", testUpstreamId),
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
		C:        requests,
		CResp:    responses,
		Port:     ports[0],
	}
}

// runTestServer starts a local server listening on a random port, that responds to requests with the provided `reply`.
// It returns the port that the server is running on, and a channel which will contain requests received by this server
func runTestServer(ctx context.Context, reply string, tlsServer UpstreamTlsRequired) (uint32, <-chan *ReceivedRequest, <-chan *ReturnedResponse) {
	return runTestServerWithHealthReply(ctx, reply, "OK", tlsServer)
}

func runTestServerWithHealthReply(ctx context.Context, reply, healthReply string, tlsServer UpstreamTlsRequired) (uint32, <-chan *ReceivedRequest, <-chan *ReturnedResponse) {
	reqChan := make(chan *ReceivedRequest, 100)
	respChan := make(chan *ReturnedResponse, 100)
	handlerFunc := func(rw http.ResponseWriter, r *http.Request) {
		var rr ReceivedRequest
		var rresp ReturnedResponse
		rr.Method = r.Method

		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
			if len(body) != 0 {
				rr.Body = body
			}
		}
		rr.Host = r.Host
		rr.URL = r.URL
		rr.Headers = r.Header

		reqChan <- &rr

		if retresp := waitIfNecessary(r); retresp != nil {
			rw.WriteHeader(retresp.Code)
			rw.Write(retresp.Body)
			rresp = *retresp
		} else if reply != "" {
			rw.Write([]byte(reply))
			rresp.Code = http.StatusOK
			rresp.Headers = rw.Header()
			rresp.Body = []byte(reply)
		} else if body != nil {
			rw.Write(body)
			rresp.Code = http.StatusOK
			rresp.Headers = rw.Header()
			rresp.Body = body
		}
		respChan <- &rresp
	}

	listener, err := getListener(tlsServer)
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
		close(reqChan)
	}()
	return uint32(port), reqChan, respChan
}

func waitIfNecessary(r *http.Request) *ReturnedResponse {
	ms := 0
	if r.URL.Query().Has("wait") {
		milliseconds := r.URL.Query().Get("wait")
		var err error
		ms, err = strconv.Atoi(milliseconds)
		if err != nil {
			return &ReturnedResponse{
				Code: http.StatusBadRequest,
				Body: []byte(err.Error()),
			}
		}
	}
	time.Sleep(time.Millisecond * time.Duration(ms))
	return nil
}

func getListener(tlsServer UpstreamTlsRequired) (net.Listener, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	if tlsServer > NO_TLS {
		fmt.Fprintln(GinkgoWriter, "test server serving tls")
		certGenFunc, keyGenFunc := helpers.Certificate, helpers.PrivateKey
		if tlsServer == MTLS {
			fmt.Fprintln(GinkgoWriter, "test server serving mtls")
			certGenFunc, keyGenFunc = helpers.MtlsCertificate, helpers.MtlsPrivateKey
		}
		cert, key := certGenFunc(), keyGenFunc()
		certs, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{certs},
		}
		if tlsServer == MTLS {
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM([]byte(cert))
			tlsConfig.ClientCAs = certPool
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
		listener = tls.NewListener(listener, tlsConfig)
	}
	return listener, nil
}

// Assertion Utilities
//
// Below are a collection of methods that can be used to assert that a configured TestUpstream successfully
// received traffic. ExpectCurlWithOffset`is the preferred utility, and includes a comment outlining how to use it

func TestUpstreamReachable(envoyPort uint32, tu *TestUpstream, rootca *string) {
	TestUpstreamReachableWithOffset(2, envoyPort, tu, rootca)
}

func TestUpstreamReachableWithOffset(offset int, envoyPort uint32, tu *TestUpstream, rootca *string) {
	body := []byte("solo.io test")

	ExpectHttpOK(body, rootca, envoyPort, "solo.io test")

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
	ExpectCurlWithOffset(
		offset+1,
		CurlRequest{
			RootCA: rootca,
			Port:   envoyPort,
			Path:   "/1",
			Body:   body,
		},
		CurlResponse{
			Message: response,
			Status:  status,
		})
}

type CurlRequest struct {
	RootCA  *string
	Port    uint32
	Path    string
	Body    []byte
	Host    string
	Headers map[string]string
}

type CurlResponse struct {
	Status  int
	Message string
}

// ExpectCurlWithOffset is the preferred utility for asserting that a request to a port was received and
// returned the expectedResponse. It provides the same functionality as the above methods, but groups parameters
// into a CurlRequest and CurlResponse object, which helps us avoid frequently updating the method parameters
// whenever new properties are required (telescoping constructor anti-pattern:
func ExpectCurlWithOffset(offset int, request CurlRequest, expectedResponse CurlResponse) {

	EventuallyWithOffset(offset+1, func(g Gomega) {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(request.Body)

		var client http.Client

		scheme := "http"
		if request.RootCA != nil {
			scheme = "https"
			caCertPool := x509.NewCertPool()
			ok := caCertPool.AppendCertsFromPEM([]byte(*request.RootCA))
			g.Expect(ok).To(BeTrue())

			tlsConfig := &tls.Config{
				RootCAs:            caCertPool,
				InsecureSkipVerify: true,
			}

			client.Transport = &http.Transport{
				TLSClientConfig: tlsConfig,
			}
		}

		requestUrl := fmt.Sprintf("%s://%s:%d%s", scheme, "localhost", request.Port, request.Path)
		req, err := http.NewRequest(http.MethodPost, requestUrl, &buf)
		g.Expect(err).NotTo(HaveOccurred())

		if request.Host != "" {
			req.Host = request.Host
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		for headerName, headerValue := range request.Headers {
			req.Header.Set(headerName, headerValue)
		}

		g.Expect(client.Do(req)).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
			StatusCode: expectedResponse.Status,
			Body:       expectedResponse.Message,
		}))
	}, "30s", "1s").Should(Succeed())
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
