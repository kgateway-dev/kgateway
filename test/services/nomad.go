package services

//import (
//	"fmt"
//	"os"
//	"os/exec"
//	"path/filepath"
//
//	"io/ioutil"
//
//	"time"
//
//	"syscall"
//
//	"bytes"
//	"strings"
//	"text/template"
//
//	"github.com/hashicorp/go-multierror"
//	"github.com/hashicorp/nomad/api"
//	"github.com/onsi/ginkgo/v2"
//	errors "github.com/rotisserie/eris"
//	"github.com/solo-io/gloo/pkg/backoff"
//	"github.com/solo-io/go-utils/log"
//	"github.com/solo-io/gloo/test/helpers"
//)
//
//const defaultNomadDockerImage = "djenriquez/nomad@sha256:31f63da9ad07b349e02f5d71bd3def416bac72cfcfd79323fd2e99abaaccdd0f"
//
//type NomadFactory struct {
//	nomadpath string
//	tmpdir    string
//}
//
//func NewNomadFactory() (*NomadFactory, error) {
//	nomadpath := os.Getenv("NOMAD_BINARY")
//
//	if nomadpath != "" {
//		return &NomadFactory{
//			nomadpath: nomadpath,
//		}, nil
//	}
//
//	// try to grab one form docker...
//	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "nomad")
//	if err != nil {
//		return nil, err
//	}
//
//	bash := fmt.Sprintf(`
//set -ex
//CID=$(docker run -d  %s /bin/sh -c exit)
//
//# just print the image sha for repoducibility
//echo "Using Nomad Image:"
//docker inspect %s -f "{{.RepoDigests}}"
//
//docker cp $CID:/bin/nomad .
//docker rm -f $CID
//    `, defaultNomadDockerImage, defaultNomadDockerImage)
//	scriptfile := filepath.Join(tmpdir, "getnomad.sh")
//
//	ioutil.WriteFile(scriptfile, []byte(bash), 0755)
//
//	cmd := exec.Command("bash", scriptfile)
//	cmd.Dir = tmpdir
//	cmd.Stdout = ginkgo.GinkgoWriter
//	cmd.Stderr = ginkgo.GinkgoWriter
//	if err := cmd.Run(); err != nil {
//		return nil, err
//	}
//
//	return &NomadFactory{
//		nomadpath: filepath.Join(tmpdir, "nomad"),
//		tmpdir:    tmpdir,
//	}, nil
//}
//
//func (ef *NomadFactory) Clean() error {
//	if ef == nil {
//		return nil
//	}
//	if ef.tmpdir != "" {
//		os.RemoveAll(ef.tmpdir)
//
//	}
//	return nil
//}
//
//type NomadInstance struct {
//	nomadpath string
//	tmpdir    string
//	cmd       *exec.Cmd
//	vault     *VaultInstance
//
//	cleanupJobs []string
//}
//
//func (ef *NomadFactory) NewNomadInstance(vault *VaultInstance) (*NomadInstance, error) {
//	// try to grab one form docker...
//	tmpdir, err := ioutil.TempDir(os.Getenv("HELPER_TMP"), "nomad")
//	if err != nil {
//		return nil, err
//	}
//
//	cmd := exec.Command(ef.nomadpath, "agent", "-dev",
//		"--vault-enabled=true",
//		"--vault-address=http://127.0.0.1:8200",
//		"--vault-token=root",
//	)
//	cmd.Dir = tmpdir
//	cmd.Stdout = ginkgo.GinkgoWriter
//	cmd.Stderr = ginkgo.GinkgoWriter
//	return &NomadInstance{
//		nomadpath: ef.nomadpath,
//		tmpdir:    tmpdir,
//		cmd:       cmd,
//		vault:     vault,
//	}, nil
//
//}
//
//func (i *NomadInstance) Silence() {
//	i.cmd.Stdout = nil
//	i.cmd.Stderr = nil
//}
//
//func (i *NomadInstance) Run() error {
//	return i.RunWithPort()
//}
//
//func (i *NomadInstance) RunWithPort() error {
//	err := i.cmd.Start()
//	if err != nil {
//		return err
//	}
//	time.Sleep(time.Millisecond * 1500)
//	return nil
//}
//
//func (i *NomadInstance) Binary() string {
//	return i.nomadpath
//}
//
//func (i *NomadInstance) Clean() error {
//	if i.cmd != nil {
//		if err := i.cmd.Process.Signal(syscall.SIGINT); err != nil {
//			return err
//		}
//		if err := i.cmd.Wait(); err != nil {
//			return err
//		}
//	}
//	if i.tmpdir != "" {
//		os.RemoveAll(i.tmpdir)
//	}
//	return nil
//}
//
//func (i *NomadInstance) Cfg() *api.Config {
//	return api.DefaultConfig()
//}
//
//func (i *NomadInstance) Exec(args ...string) (string, error) {
//	cmd := exec.Command(i.nomadpath, args...)
//	cmd.Env = os.Environ()
//	// disable DEBUG=1 from getting through to nomad
//	for i, pair := range cmd.Env {
//		if strings.HasPrefix(pair, "DEBUG") {
//			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
//			break
//		}
//	}
//	out, err := cmd.CombinedOutput()
//	if err != nil {
//		err = fmt.Errorf("%s (%v)", out, err)
//	}
//	return string(out), err
//}
//
//func (i *NomadInstance) SetupNomadForE2eTest(envoyPath, outputDirectory string, buildBinaries bool) error {
//	if buildBinaries {
//		if _, err := downloadNats(outputDirectory); err != nil {
//			return errors.Wrap(err, "downloading nats")
//		}
//		if _, err := downloadPetstore(outputDirectory); err != nil {
//			return errors.Wrap(err, "downloading petstore")
//		}
//
//		if err := helpers.BuildBinaries(outputDirectory, false); err != nil {
//			return errors.Wrap(err, "building binaries")
//		}
//	}
//	nomadResourcesDir := filepath.Join(helpers.NomadE2eDirectory(), "nomad_resources")
//
//	data := &struct {
//		OutputDirectory string
//		EnvoyPath       string
//	}{OutputDirectory: outputDirectory, EnvoyPath: envoyPath}
//
//	tmpl, err := template.New("Test_Resources").ParseFiles(filepath.Join(nomadResourcesDir, "install.nomad.tmpl"))
//	if err != nil {
//		return errors.Wrap(err, "parsing template from install.nomad.tmpl")
//	}
//
//	buf := &bytes.Buffer{}
//	if err := tmpl.ExecuteTemplate(buf, "install.nomad.tmpl", data); err != nil {
//		return errors.Wrap(err, "executing template")
//	}
//
//	err = ioutil.WriteFile(filepath.Join(nomadResourcesDir, "install.nomad"), buf.Bytes(), 0644)
//	if err != nil {
//		return errors.Wrap(err, "writing generated test resources template")
//	}
//
//	tmpl, err = template.New("Test_Resources").ParseFiles(filepath.Join(nomadResourcesDir, "testing-resources.nomad.tmpl"))
//	if err != nil {
//		return errors.Wrap(err, "parsing template from testing-resources.nomad.tmpl")
//	}
//
//	buf = &bytes.Buffer{}
//	if err := tmpl.ExecuteTemplate(buf, "testing-resources.nomad.tmpl", data); err != nil {
//		return errors.Wrap(err, "executing template")
//	}
//
//	err = ioutil.WriteFile(filepath.Join(nomadResourcesDir, "testing-resources.nomad"), buf.Bytes(), 0644)
//	if err != nil {
//		return errors.Wrap(err, "writing generated test resources template")
//	}
//
//	_, err = i.vault.Exec("policy", "write", "-address=http://127.0.0.1:8200", "gloo", filepath.Join(nomadResourcesDir, "gloo-policy.hcl"))
//	if err != nil {
//		return errors.Wrap(err, "setting vault policy")
//	}
//
//	err = backoff.WithBackoff(func() error {
//		// test stuff first
//		if _, err := i.Exec("run", filepath.Join(nomadResourcesDir, "testing-resources.nomad")); err != nil {
//			return errors.Wrapf(err, "creating nomad resource from testing-resources.nomad")
//		}
//		i.cleanupJobs = append(i.cleanupJobs, "testing-resources")
//		return nil
//	}, nil)
//
//	if err != nil {
//		return errors.Wrap(err, "creating job for testing-resources")
//	}
//
//	if err := i.waitJobRunning("testing-resources"); err != nil {
//		return errors.Wrap(err, "waiting for job to start")
//	}
//
//	if _, err := i.Exec("run", filepath.Join(nomadResourcesDir, "install.nomad")); err != nil {
//		return errors.Wrapf(err, "creating nomad resource from install.nomad")
//	}
//
//	i.cleanupJobs = append(i.cleanupJobs, "gloo")
//	if err := i.waitJobRunning("gloo"); err != nil {
//		return errors.Wrap(err, "waiting for job to start")
//	}
//
//	var ingressAddr string
//
//	err = backoff.WithBackoff(func() error {
//		addr, err := helpers.ConsulServiceAddress("ingress", "admin")
//		if err != nil {
//			return errors.Wrap(err, "getting ingress addr")
//		}
//		ingressAddr = addr
//		return nil
//	}, nil)
//
//	if err != nil {
//		return errors.Wrap(err, "creating getting ingress addr")
//	}
//	_, err = helpers.Curl(ingressAddr, helpers.CurlOpts{Path: "/logging?config=debug"})
//	return err
//}
//
//func (i *NomadInstance) waitJobRunning(name string) error {
//	return i.waitJobStatus(name, "running")
//}
//
//func (i *NomadInstance) waitJobStatus(job, status string) error {
//	cfg := i.Cfg()
//	client, err := api.NewClient(cfg)
//	if err != nil {
//		return err
//	}
//	statusFunc := func() (string, error) {
//		info, _, err := client.Jobs().Info(job, nil)
//		if err != nil {
//			return "", err
//		}
//		if *info.Stop {
//			return "stopped", nil
//		}
//		return *info.Status, nil
//	}
//
//	timeout := time.Second * 20
//	interval := time.Millisecond * 1000
//	tick := time.Tick(interval)
//
//	log.Debugf("waiting %v for pod %v to be %v...", timeout, job, status)
//	for {
//		select {
//		case <-time.After(timeout):
//			return fmt.Errorf("timed out waiting for %v to be %v", job, status)
//		case <-tick:
//			out, err := statusFunc()
//			if err != nil {
//				return fmt.Errorf("failed getting status: %v", err)
//			}
//			if strings.Contains(out, "dead") || strings.Contains(out, "failed") {
//				out, _ = i.Exec("status", job)
//				return errors.Errorf("%v in dead with logs %v", job, out)
//			}
//			if out == status {
//				return nil
//			}
//		}
//	}
//}
//
//func (i *NomadInstance) TeardownNomadE2e() error {
//	var result *multierror.Error
//	for _, job := range i.cleanupJobs {
//
//		out, err := i.Exec("job", "stop", "-purge", job)
//		if err != nil {
//			multierror.Append(result, errors.Wrapf(err, "stop job failed: %v", out))
//		}
//	}
//	return result.ErrorOrNil()
//}
//func (i *NomadInstance) Logs(job, task string) (string, error) {
//	allocId, err := i.getAllocationId(job)
//	if err != nil {
//		return "", err
//	}
//	stdout, err := i.Exec("logs", allocId, task)
//	if err != nil {
//		return "", err
//	}
//	stderr, err := i.Exec("logs", "-stderr", allocId, task)
//	return stdout + "\n\n" + stderr, nil
//}
//
//func (i *NomadInstance) getAllocationId(job string) (string, error) {
//	cfg := i.Cfg()
//	client, err := api.NewClient(cfg)
//	if err != nil {
//		return "", err
//	}
//	allocs, _, err := client.Jobs().Allocations("gloo", false, nil)
//	if err != nil {
//		return "", err
//	}
//	if len(allocs) < 1 {
//		return "", errors.Errorf("expected at least 1 allocation, got %v", len(allocs))
//	}
//	return allocs[0].ID, nil
//}
