package e2e

import (
	"os/exec"
	"regexp"
	"time"

	"os"
	"path/filepath"

	"strings"

	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/crd"
	. "github.com/solo-io/gloo/test/helpers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// namespace needs to be set here or tests will break
	namespace = getNamespace()
)

func getNamespace() string {
	rand.Seed(time.Now().UTC().UnixNano())
	return RandString(6)
}

var gloo storage.Interface
var kube kubernetes.Interface

var _ = Describe("Kubernetes Deployment", func() {
	BeforeSuite(func() {
		log.Printf("USING IMAGE TAG %v", ImageTag)

		// are we on minikube? set docket env vars and push to false
		push := true
		debug := false
		if setupMinikubeEnvVars() {
			push = false
		}
		if os.Getenv("DEBUG_IMAGES") =="1" {
			debug = true
		}

		log.Debugf("SetupKubeForE2eTest: push =  %v \t namespace = %v", push, namespace)

		err := SetupKubeForE2eTest(namespace, true, push, debug)
		Must(err)
		kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		masterUrl := ""
		Must(err)
		cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
		Must(err)
		gloo, err = crd.NewStorage(cfg, namespace, time.Minute)
		Must(err)
		kube, err = kubernetes.NewForConfig(cfg)
		Must(err)
	})
	AfterSuite(func() {
		TeardownKubeE2E(namespace)
	})
})

type curlOpts struct {
	protocol string
	path     string
	method   string
	host     string
	service  string
	caFile   string
	body     string
	headers  map[string]string
	port     int
}

func curlEventuallyShouldRespond(opts curlOpts, substr string, timeout ...time.Duration) {
	t := time.Second * 20
	if len(timeout) > 0 {
		t = timeout[0]
	}
	// for some useful-ish output
	tick := time.Tick(t / 8)
	Eventually(func() string {
		res, err := curl(opts)
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
	}, t,"5s").Should(ContainSubstring(substr))
}

func curl(opts curlOpts) (string, error) {
	args := []string{"curl", "-v"}

	if opts.method != "GET" && opts.method != "" {
		args = append(args, "-X"+opts.method)
	}
	if opts.host != "" {
		args = append(args, "-H", "Host: "+opts.host)
	}
	if opts.caFile != "" {
		args = append(args, "--cacert", opts.caFile)
	}
	if opts.body != "" {
		args = append(args, "-H", "Content-Type: application/json")
		args = append(args, "-d", opts.body)
	}
	for h, v := range opts.headers {
		args = append(args, "-H", fmt.Sprintf("%v: %v", h, v))
	}
	port := opts.port
	if port == 0 {
		port = 8080
	}
	protocol := opts.protocol
	if protocol == "" {
		protocol = "http"
	}
	service := opts.service
	if service == "" {
		service = "test-ingress"
	}
	args = append(args, fmt.Sprintf("%v://%s:%v%s", protocol, service, port, opts.path))
	log.Debugf("running: curl %v", strings.Join(args, " "))
	return TestRunner(args...)
}

func setupMinikubeEnvVars() bool {
	// are we in minikube?
	out, _ := exec.Command("kubectl", "config", "current-context").CombinedOutput()
	if strings.Contains(string(out), "minikube") {
		return setupEnvFromMinikube()
	}
	return false
}

var lineregex = regexp.MustCompile("export (\\S+)=\"(.+)\"")

func setupEnvFromMinikube() bool {
	out, err := exec.Command("minikube", "docker-env", "--shell", "bash").CombinedOutput()
	if err != nil {
		return false
	}
	outs := string(out)
	varsset := false
	lines := strings.Split(outs, "\n")
	const prefix = "export "
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			continue
		}
		matches := lineregex.FindStringSubmatch(line)
		if matches != nil {
			varsset = true
			log.Debugf("Settings var: %v %v", matches[1], matches[2])
			os.Setenv(matches[1], matches[2])
		}
	}
	return varsset
}
