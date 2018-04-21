package utils

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

	"fmt"
	"os/exec"
	"strings"

	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/backoff"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

func SetupNomadForE2eTest(nomadInstance *localhelpers.NomadInstance, buildImages bool) error {
	if buildImages {
		if err := helpers.BuildPushContainers(false, false); err != nil {
			return err
		}
	}
	nomadResourcesDir := filepath.Join(helpers.NomadE2eDirectory(), "nomad_resources")

	envoyImageTag := os.Getenv("ENVOY_IMAGE_TAG")
	if envoyImageTag == "" {
		log.Warnf("no ENVOY_IMAGE_TAG specified, defaulting to latest")
		envoyImageTag = "latest"
	}

	data := &struct {
		ImageTag      string
		EnvoyImageTag string
	}{ImageTag: helpers.ImageTag(), EnvoyImageTag: envoyImageTag}

	tmpl, err := template.New("Test_Resources").ParseFiles(filepath.Join(nomadResourcesDir, "install.nomad.tmpl"))
	if err != nil {
		return errors.Wrap(err, "parsing template from install.nomad.tmpl")
	}

	buf := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, "install.nomad.tmpl", data); err != nil {
		return errors.Wrap(err, "executing template")
	}

	err = ioutil.WriteFile(filepath.Join(nomadResourcesDir, "install.nomad"), buf.Bytes(), 0644)
	if err != nil {
		return errors.Wrap(err, "writing generated test resources template")
	}

	tmpl, err = template.New("Test_Resources").ParseFiles(filepath.Join(nomadResourcesDir, "testing-resources.nomad.tmpl"))
	if err != nil {
		return errors.Wrap(err, "parsing template from testing-resources.nomad.tmpl")
	}

	buf = &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, "testing-resources.nomad.tmpl", data); err != nil {
		return errors.Wrap(err, "executing template")
	}

	err = ioutil.WriteFile(filepath.Join(nomadResourcesDir, "testing-resources.nomad"), buf.Bytes(), 0644)
	if err != nil {
		return errors.Wrap(err, "writing generated test resources template")
	}

	_, err = Vault("policy", "write", "-address=http://127.0.0.1:8200", "gloo", filepath.Join(nomadResourcesDir, "gloo-policy.hcl"))
	if err != nil {
		return errors.Wrap(err, "setting vault policy")
	}

	backoff.WithBackoff(func() error {
		// test stuff first
		if _, err := Nomad("run", filepath.Join(nomadResourcesDir, "testing-resources.nomad")); err != nil {
			return errors.Wrapf(err, "creating nomad resource from testing-resources.nomad")
		}
		return nil
	}, make(chan struct{}))

	if err := waitJobRunning(nomadInstance, "testing-resources"); err != nil {
		return errors.Wrap(err, "waiting for job to start")
	}

	if _, err := Nomad("run", filepath.Join(nomadResourcesDir, "install.nomad")); err != nil {
		return errors.Wrapf(err, "creating nomad resource from install.nomad")
	}

	if err := waitJobRunning(nomadInstance, "gloo"); err != nil {
		return errors.Wrap(err, "waiting for job to start")
	}

	var ingressAddr string

	backoff.WithBackoff(func() error {
		addr, err := helpers.ConsulServiceAddress("ingress", "admin")
		if err != nil {
			return errors.Wrap(err, "getting ingress addr")
		}
		ingressAddr = addr
		return nil
	}, make(chan struct{}))

	_, err = helpers.Curl(ingressAddr, helpers.CurlOpts{Path: "/logging?config=debug"})
	return err
}

func TeardownNomadE2e() error {
	out, err := Nomad("job", "stop", "-purge", "gloo")
	if err != nil {
		return errors.Wrapf(err, "stop job failed: %v", out)
	}
	out, err = Nomad("job", "stop", "-purge", "testing-resources")
	if err != nil {
		return errors.Wrapf(err, "stop job failed: %v", out)
	}
	return nil
}

func Nomad(args ...string) (string, error) {
	cmd := exec.Command("nomad", args...)
	cmd.Env = os.Environ()
	// disable DEBUG=1 from getting through to nomad
	for i, pair := range cmd.Env {
		if strings.HasPrefix(pair, "DEBUG") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
			break
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s (%v)", out, err)
	}
	return string(out), err
}

func Vault(args ...string) (string, error) {
	cmd := exec.Command("vault", args...)
	cmd.Env = os.Environ()
	// disable DEBUG=1 from getting through to nomad
	for i, pair := range cmd.Env {
		if strings.HasPrefix(pair, "DEBUG") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
			break
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s (%v)", out, err)
	}
	return string(out), err
}

func Logs(nomadInstance *localhelpers.NomadInstance, job, task string) (string, error) {
	allocId, err := getAllocationId(nomadInstance, job)
	if err != nil {
		return "", err
	}
	stdout, err := Nomad("logs", allocId, task)
	if err != nil {
		return "", err
	}
	stderr, err := Nomad("logs", "-stderr", allocId, task)
	return stdout + "\n\n" + stderr, nil
}

func getAllocationId(nomadInstance *localhelpers.NomadInstance, job string) (string, error) {
	cfg := nomadInstance.Cfg()
	client, err := api.NewClient(cfg)
	if err != nil {
		return "", err
	}
	allocs, _, err := client.Jobs().Allocations("gloo", false, nil)
	if err != nil {
		return "", err
	}
	if len(allocs) < 1 {
		return "", errors.Errorf("expected at least 1 allocation, got %v", len(allocs))
	}
	return allocs[0].ID, nil
}

func waitJobRunning(nomadInstance *localhelpers.NomadInstance, name string) error {
	return waitJobStatus(nomadInstance, name, "running")
}

func waitJobStatus(nomadInstance *localhelpers.NomadInstance, job, status string) error {
	cfg := nomadInstance.Cfg()
	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}
	statusFunc := func() (string, error) {
		info, _, err := client.Jobs().Info(job, nil)
		if err != nil {
			return "", err
		}
		if *info.Stop {
			return "stopped", nil
		}
		return *info.Status, nil
	}

	timeout := time.Second * 20
	interval := time.Millisecond * 1000
	tick := time.Tick(interval)

	log.Debugf("waiting %v for pod %v to be %v...", timeout, job, status)
	for {
		select {
		case <-time.After(timeout):
			return fmt.Errorf("timed out waiting for %v to be %v", job, status)
		case <-tick:
			out, err := statusFunc()
			if err != nil {
				return fmt.Errorf("failed getting status: %v", err)
			}
			if strings.Contains(out, "dead") || strings.Contains(out, "failed") {
				out, _ = Nomad("status", job)
				return errors.Errorf("%v in dead with logs %v", job, out)
			}
			if out == status {
				return nil
			}
		}
	}
}
