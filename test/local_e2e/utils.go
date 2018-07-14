package local_e2e

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"context"
	"io/ioutil"
	"time"

	"path/filepath"

	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/local_e2e/test_grpc_service"
)

type ReceivedRequest struct {
	Method      string
	Body        []byte
	GRPCRequest proto.Message
}
type TestUpstream struct {
	Upstream *v1.Upstream
	C        <-chan *ReceivedRequest
}

func RunTestServer(ctx context.Context) (uint32, <-chan *ReceivedRequest) {
	bodychan := make(chan *ReceivedRequest, 100)
	handlerfunc := func(rw http.ResponseWriter, r *http.Request) {
		var rr ReceivedRequest
		rr.Method = r.Method
		if r.Body != nil {
			body, _ := ioutil.ReadAll(r.Body)
			r.Body.Close()
			if len(body) != 0 {
				rr.Body = body
			}
		}
		bodychan <- &rr
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	addr := listener.Addr().String()
	_, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}

	port, err := strconv.Atoi(portstr)
	if err != nil {
		panic(err)
	}

	handler := http.HandlerFunc(handlerfunc)
	go func() {
		h := &http.Server{Handler: handler}
		go func() {
			if err := h.Serve(listener); err != nil {
				if err != http.ErrServerClosed {
					panic(err)
				}
			}
		}()

		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		h.Shutdown(ctx)
		cancel()
	}()
	return uint32(port), bodychan
}

var id = 0

func NewTestHttpUpstream(ctx context.Context, addr string) *TestUpstream {
	backendport, responses := RunTestServer(ctx)
	return newTestUpstream(addr, backendport, responses)
}

func NewTestGRPCUpstream(addr string, glooFilesDir string) *TestUpstream {
	srv := testgrpcservice.RunServer()
	received := make(chan *ReceivedRequest)
	go func() {
		for r := range srv.C {
			received <- &ReceivedRequest{GRPCRequest: r}
		}
	}()
	protobytes, err := ioutil.ReadFile(filepath.Join(helpers.LocalE2eDirectory(), "test_grpc_service", "descriptors", "proto.pb"))
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(filepath.Join(glooFilesDir, "proto.pb"), protobytes, 0644); err != nil {
		panic(err)
	}
	us := newTestUpstream(addr, srv.Port, received)
	us.Upstream.ServiceInfo = &v1.ServiceInfo{
		Type: grpc.ServiceTypeGRPC,
		Properties: grpc.EncodeServiceProperties(grpc.ServiceProperties{
			GrpcServiceNames:   []string{"TestService"},
			DescriptorsFileRef: "proto.pb",
		}),
	}
	return us
}

func newTestUpstream(addr string, port uint32, responses <-chan *ReceivedRequest) *TestUpstream {
	serviceSpec := &static.UpstreamSpec{
		Hosts: []*static.Host{{
			Addr: addr,
			Port: port,
		}},
	}
	id += 1
	u := &v1.Upstream{
		Name: fmt.Sprintf("local-%d", id),
		Type: "service",
		Spec: static.EncodeUpstreamSpec(serviceSpec),
	}

	return &TestUpstream{
		Upstream: u,
		C:        responses,
	}
}
