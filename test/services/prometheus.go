package services

import (
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"os"
	"path/filepath"

	"io/ioutil"

	"github.com/onsi/gomega/gexec"
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

/**

https://github.com/prometheus/prometheus/blob/master/config/config.go

*/

const configTemplate = `
global:
  scrape_interval: 100ms
scrape_configs:
  - job_name: 'envoy'
    metrics_path: /stats/prometheus
    file_sd_configs:
    - files:
      - %s/*.yaml
`

type StaticConfigs []*targetgroup.Group

type PrometheusFactory struct {
	instances []*PrometheusInstance
}

func NewPrometheusFactory() (*PrometheusFactory, error) {
	return &PrometheusFactory{}, nil
}

func (pf *PrometheusFactory) Clean() error {
	if pf == nil {
		return nil
	}
	instances := pf.instances
	pf.instances = nil
	for _, pi := range instances {
		pi.Clean()
	}
	return nil
}

func (pf *PrometheusFactory) NewPrometheusInstance() *PrometheusInstance {
	instance := newPrometheusInstance(9090)
	pf.instances = append(pf.instances, instance)
	return instance
}

type PrometheusInstance struct {
	Port          int32
	cmd           *exec.Cmd
	tmpdir        string
	staticconfigs string
}

func newPrometheusInstance(port int32) *PrometheusInstance {

	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "prometheus")
	Expect(err).NotTo(HaveOccurred())

	// create config dir
	staticconfigs := filepath.Join(tmpdir, "static-configs")
	err = os.Mkdir(staticconfigs, 0755)
	Expect(err).NotTo(HaveOccurred())

	// create data dir
	datadir := filepath.Join(tmpdir, "data")
	err = os.Mkdir(datadir, 0755)
	Expect(err).NotTo(HaveOccurred())

	cfg := fmt.Sprintf(configTemplate, staticconfigs)
	promyaml := filepath.Join(tmpdir, "prometheus.yml")
	err = ioutil.WriteFile(promyaml, []byte(cfg), 0400)
	Expect(err).NotTo(HaveOccurred())

	cmd := exec.Command("prometheus", "--config.file="+promyaml, fmt.Sprintf("--web.listen-address=127.0.0.1:%d", port))
	cmd.Dir = datadir
	_, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())

	// write config
	return &PrometheusInstance{
		Port:          port,
		tmpdir:        tmpdir,
		cmd:           cmd,
		staticconfigs: staticconfigs,
	}
}

func (pi *PrometheusInstance) AddMesh(m *QuoteUnquoteMesh) {
	for _, ei := range m.Envoys {
		pi.AddEnvoy(ei)
	}
}

func (pi *PrometheusInstance) AddEnvoy(ei *EnvoyInstance) {
	fname := filepath.Join(pi.staticconfigs, fmt.Sprintf("%d.yaml", ei.AdminPort))
	jase := `
[
  {
    "targets": [ "127.0.0.1:%d" ],
    "labels": {}
  },
]
`

	envoyDs := fmt.Sprintf(jase, ei.AdminPort)

	err := ioutil.WriteFile(fname, []byte(envoyDs), 0400)
	Expect(err).NotTo(HaveOccurred())
}

func (pi *PrometheusInstance) Clean() error {
	if pi.cmd != nil {
		pi.cmd.Process.Kill()
		pi.cmd.Wait()
	}

	if pi.tmpdir != "" {
		os.RemoveAll(pi.tmpdir)
	}
	return nil
}

func (pi *PrometheusInstance) Client() promv1.API {
	client, err := promapi.NewClient(promapi.Config{Address: fmt.Sprintf("http://localhost:%d", pi.Port)})
	Expect(err).NotTo(HaveOccurred())
	return promv1.NewAPI(client)
}
