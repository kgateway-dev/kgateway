package local_e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HappyPath EDS", func() {

	It("Receive proxied from eds", func() {
		fmt.Fprintln(GinkgoWriter, "Running Envoy")
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		fmt.Fprintln(GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()
		fmt.Fprintln(GinkgoWriter, "Envoy Port: ", envoyPort)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		fmt.Fprintln(GinkgoWriter, "adding upstream")
		tu := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// fake eds
		tu.Upstream.Type = fake.UpstreamTypeFake

		fmt.Fprintln(GinkgoWriter, tu.Upstream)
		err = glooInstance.AddUpstream(tu.Upstream)
		Expect(err).NotTo(HaveOccurred())

		v := &v1.VirtualService{
			Name: "default",
			Routes: []*v1.Route{{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: tu.Upstream.Name,
						},
					},
				},
			}},
		}

		fmt.Fprintln(GinkgoWriter, "adding virtual service")
		err = glooInstance.AddvService(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("solo.io test")

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			_, err = http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
			return err
		}, 90, 1).Should(BeNil())

		expectedResponse := &ReceivedRequest{
			Method: "POST",
			Body:   body,
		}
		Eventually(tu.C, "2s").Should(Receive(Equal(expectedResponse)))

	})

	It("Receive proxied from eds after non eds", func() {
		fmt.Fprintln(GinkgoWriter, "Running Envoy")
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		fmt.Fprintln(GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()
		fmt.Fprintln(GinkgoWriter, "Envoy Port: ", envoyPort)

		doRegular()
		doRegular()
		doRegular()
		doRegular()
		doEDS(1)
		doEDS(2)
		doEDS(3)
		doEDS(4)
		doEDS(8)
	})

	It("Receive proxied from eds after non eds zbam mode", func() {
		fmt.Fprintln(GinkgoWriter, "Running Envoy")
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		Eventually(envoyInstance.DebugMode, "60s", "1s").Should(BeNil())

		fmt.Fprintln(GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 2; i++ {
			dozbam()
		}
	})

})

func doRegular() {
	envoyPort := glooInstance.EnvoyPort()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tu := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

	err := glooInstance.AddUpstream(tu.Upstream)
	Expect(err).NotTo(HaveOccurred())
	defer glooInstance.DeleteUpstream(tu.Upstream.Name)

	v := &v1.VirtualService{
		Name: "default",
		Routes: []*v1.Route{{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: tu.Upstream.Name,
					},
				},
			},
		}},
	}

	fmt.Fprintln(GinkgoWriter, "adding virtual service")
	err = glooInstance.AddvService(v)
	defer glooInstance.DeletevService(v.Name)
	Expect(err).NotTo(HaveOccurred())
	body := []byte("solo.io test")
	time.Sleep(5 * time.Second)

	// wait for envoy to start receiving request
	Eventually(func() error {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)
		_, err = http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
		return err
	}, 90, 1).Should(BeNil())

}

func dozbam() {

	envoyPort := glooInstance.EnvoyPort()
	fmt.Fprintln(GinkgoWriter, "Envoy Port: ", envoyPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tu := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

	err := glooInstance.AddUpstream(tu.Upstream)
	Expect(err).NotTo(HaveOccurred())

	v := &v1.VirtualService{
		Name: "default",
		Routes: []*v1.Route{{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: tu.Upstream.Name,
					},
				},
			},
		}},
	}

	fmt.Fprintln(GinkgoWriter, "adding virtual service")
	err = glooInstance.AddvService(v)
	defer glooInstance.DeletevService(v.Name)
	Expect(err).NotTo(HaveOccurred())
	body := []byte("solo.io test")
	time.Sleep(5 * time.Second)

	// wait for envoy to start receiving request
	Eventually(func() error {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)
		_, err = http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
		return err
	}, 90, 1).Should(BeNil())

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	tutmp := NewTestHttpUpstream(ctx2, envoyInstance.LocalAddr())
	// fake eds
	tutmp.Upstream.Type = fake.UpstreamTypeFake
	fmt.Fprintln(GinkgoWriter, tutmp.Upstream)
	// Add and delete - no sleep;
	err = glooInstance.AddUpstream(tutmp.Upstream)
	cancel()
	// zbam two of them
	// TODO: try not to delete
	defer glooInstance.DeleteUpstream(tu.Upstream.Name)

	defer glooInstance.DeleteUpstream(tutmp.Upstream.Name)

	glooInstance.DeletevService(v.Name)
	v = &v1.VirtualService{
		Name: "default",
		Routes: []*v1.Route{{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: tutmp.Upstream.Name,
					},
				},
			},
		},

			{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/2"},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &v1.UpstreamDestination{
							Name: tu.Upstream.Name,
						},
					},
				},
			},
		},
	}
	err = glooInstance.AddvService(v)
	Expect(err).NotTo(HaveOccurred())
	defer glooInstance.DeletevService(v.Name)

	time.Sleep(1 * time.Second)

	body = []byte("solo.io test")

	// wait for envoy to start receiving request
	Eventually(func() error {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)
		resp, err := http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 300 {
			return errors.New("bad response")
		}
		return nil
	}, "90000s", 1).Should(BeNil())

	expectedResponse := &ReceivedRequest{
		Method: "POST",
		Body:   body,
	}
	Eventually(tutmp.C, "5s", time.Second/2).Should(Receive(Equal(expectedResponse)))

}

func doEDS(howmany int) {
	envoyPort := glooInstance.EnvoyPort()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Fprintln(GinkgoWriter, "adding upstream")

	var tu *TestUpstream
	for i := 0; i < howmany; i++ {

		tutmp := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		tu = tutmp
		// fake eds
		tutmp.Upstream.Type = fake.UpstreamTypeFake
		fmt.Fprintln(GinkgoWriter, tutmp.Upstream)
		err := glooInstance.AddUpstream(tutmp.Upstream)
		Expect(err).NotTo(HaveOccurred())
		defer glooInstance.DeleteUpstream(tutmp.Upstream.Name)
	}

	v := &v1.VirtualService{
		Name: "default",
		Routes: []*v1.Route{{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: tu.Upstream.Name,
					},
				},
			},
		}},
	}

	fmt.Fprintln(GinkgoWriter, "adding virtual service")
	err := glooInstance.AddvService(v)
	Expect(err).NotTo(HaveOccurred())
	defer glooInstance.DeletevService(v.Name)

	time.Sleep(1 * time.Second)

	body := []byte("solo.io test")

	// wait for envoy to start receiving request
	Eventually(func() error {
		// send a request with a body
		var buf bytes.Buffer
		buf.Write(body)
		resp, err := http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 300 {
			return errors.New("bad response")
		}
		return nil
	}, 90, 1).Should(BeNil())

	expectedResponse := &ReceivedRequest{
		Method: "POST",
		Body:   body,
	}
	Eventually(tu.C, "5s", time.Second/2).Should(Receive(Equal(expectedResponse)))
}

// reset; DEBUG=1 ENVOY_IMAGE_TAG="v0.1.6-131" ginkgo  -v
// echo 0 | sudo tee /proc/sys/kernel/yama/ptrace_scope
// reset; USE_GDBSERVER_ENVOY=2345 DEBUG=1 ENVOY_IMAGE_TAG="v0.1.6-131" ginkgo  -v
/*

reset; ENVOY_BINARY=/home/yuval/nobackup/src/envoy/bazel-bin/source/exe/envoy-static  DEBUG=1 ENVOY_IMAGE_TAG="v0.1.6-131" ginkgo  -v
reset; USE_GDBSERVER_ENVOY=2345 ENVOY_BINARY=/home/yuval/nobackup/src/envoy/bazel-bin/source/exe/envoy-static  DEBUG=1 ENVOY_IMAGE_TAG="v0.1.6-131" ginkgo  -v
reset; USE_DEBUGGER_ENVOY=1 ENVOY_BINARY=/home/yuval/.cache/bazel/_bazel_yuval/f99e15bcd233f918bee00107c646e5b0/execroot/envoy/bazel-out/k8-dbg/bin/source/exe/envoy-static  DEBUG=1 ENVOY_IMAGE_TAG="v0.1.6-131" ginkgo  -v


*/
