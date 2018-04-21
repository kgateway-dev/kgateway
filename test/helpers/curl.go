package helpers

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
)

type CurlOpts struct {
	Protocol string
	Path     string
	Method   string
	Host     string
	CaFile   string
	Body     string
	Headers  map[string]string
}

func CurlEventuallyShouldRespond(opts CurlOpts, substr string, timeout ...time.Duration) {
	t := time.Second * 20
	if len(timeout) > 0 {
		t = timeout[0]
	}

	scheme := "http"
	if opts.Protocol != "" {
		scheme = opts.Protocol
	}

	addr, err := ConsulServiceAddress("ingress", scheme)
	Expect(err).NotTo(HaveOccurred())

	// for some useful-ish output
	tick := time.Tick(t / 8)
	Eventually(func() string {
		res, err := Curl(addr, opts)
		if err != nil {
			res = err.Error()
		}
		select {
		default:
			break
		case <-tick:
			log.GreyPrintf("running: %v\nwant %v\nhave: %s", opts, substr, res)
		}
		if strings.Contains(res, substr) {
			log.GreyPrintf("success: %v", res)
		}
		return res
	}, t, "5s").Should(ContainSubstring(substr))
}

func Curl(addr string, opts CurlOpts) (string, error) {
	args := []string{"-v", "--connect-timeout", "10", "--max-time", "10"}

	if opts.Method != "GET" && opts.Method != "" {
		args = append(args, "-X"+opts.Method)
	}
	if opts.Host != "" {
		args = append(args, "-H", "Host: "+opts.Host)
	}
	if opts.CaFile != "" {
		args = append(args, "--cacert", opts.CaFile)
	}
	if opts.Body != "" {
		args = append(args, "-H", "Content-Type: application/json")
		args = append(args, "-d", opts.Body)
	}
	for h, v := range opts.Headers {
		args = append(args, "-H", fmt.Sprintf("%v: %v", h, v))
	}
	protocol := opts.Protocol
	if protocol == "" {
		protocol = "http"
	}
	split := strings.Split(addr, ":")
	ip := split[0]
	port := split[1]
	args = append(args, "--resolve", fmt.Sprintf("%s:%s:%s", "test-ingress", port, ip))
	args = append(args, fmt.Sprintf("%s://%s:%s%s", protocol, "test-ingress", port, opts.Path))
	log.Debugf("running: curl %v", strings.Join(args, " "))
	return Exec("curl", args...)
}
