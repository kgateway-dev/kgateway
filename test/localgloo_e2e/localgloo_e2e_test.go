package localgloo_e2e_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	controlplanebootstrap "github.com/solo-io/gloo/pkg/control-plane/bootstrap"
	functiondiscoveryopts "github.com/solo-io/gloo/pkg/function-discovery/options"
	upstreamdiscbootstrap "github.com/solo-io/gloo/pkg/upstream-discovery/bootstrap"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/localgloo"
)

var (
	tmpDir  string
	err     error
	xdsPort = 8081

	baseOpts         bootstrap.Options
	controlPlaneOpts = controlplanebootstrap.Options{
		IngressOptions: controlplanebootstrap.IngressOptions{
			BindAddress: "::",
			Port:        uint32(xdsPort),
			SecurePort:  uint32(xdsPort + 1),
		},
	}
	upstreamDiscoveryOpts  = upstreamdiscbootstrap.Options{
		UpstreamDiscoveryOptions: upstreamdiscbootstrap.UpstreamDiscoveryOptions{
			EnableDiscoveryForConsul: true,
		},
	}
	functionDiscoveryOpts = functiondiscoveryopts.DiscoveryOptions{
		//AutoDiscoverSwagger:   true,
		//AutoDiscoverNATS:      true,
		//AutoDiscoverFaaS:      true,
		//AutoDiscoverFission:   true,
		//AutoDiscoverProjectFn: true,
		//AutoDiscoverGRPC:      true,
	}
)
var _ = BeforeSuite(func() {
	tmpDir, err = ioutil.TempDir("", "localgloo-test")
	Expect(err).To(BeNil())
	configDir := tmpDir
	filesDir := filepath.Join(tmpDir, "files")
	secretsDir := filepath.Join(tmpDir, "secrets")
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(filesDir, 0755)
	os.MkdirAll(secretsDir, 0755)
	baseOpts.ConfigStorageOptions.Type = "file"
	baseOpts.FileStorageOptions.Type = "file"
	baseOpts.SecretStorageOptions.Type = "file"
	baseOpts.FileOptions.ConfigDir = configDir
	baseOpts.FileOptions.SecretDir = secretsDir
	baseOpts.FileOptions.FilesDir = filesDir
	baseOpts.ConfigStorageOptions.SyncFrequency = time.Second
	baseOpts.FileStorageOptions.SyncFrequency = time.Second
	baseOpts.SecretStorageOptions.SyncFrequency = time.Second
	controlPlaneOpts.Options = baseOpts
	upstreamDiscoveryOpts.Options = baseOpts

	stop := make(chan struct{})
	go localgloo.Run(stop, xdsPort, baseOpts, controlPlaneOpts, upstreamDiscoveryOpts, functionDiscoveryOpts)
})
var _ = AfterSuite(func() {
	os.RemoveAll(tmpDir)
})
