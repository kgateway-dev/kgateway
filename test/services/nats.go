package services

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
)

type NatsStreamingFactory struct {
	nats   string
	tmpdir string
}

func downloadNats(destDir string) (string, error) {
	// get us some natsss
	// TODO (celsosantos): Add ARM64 support
	natsurl := fmt.Sprintf("https://github.com/nats-io/nats-streaming-server/releases/download/v0.9.0/nats-streaming-server-v0.9.0-%s-amd64.zip",
		runtime.GOOS)

	resp, err := http.Get(natsurl)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	if resp.Body == nil {
		return "", errors.New("no body")
	}
	defer resp.Body.Close()
	io.Copy(&buffer, resp.Body)

	rederat := bytes.NewReader(buffer.Bytes())
	// unzip
	r, err := zip.NewReader(rederat, int64(rederat.Len()))
	if err != nil {
		return "", err
	}

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "nats-streaming-server") {
			natsfile := filepath.Join(destDir, "nats-streaming-server")
			natsout, err := os.OpenFile(natsfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			defer natsout.Close()
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			_, err = io.Copy(natsout, rc)
			if err != nil {
				return "", err
			}
			return natsfile, nil

		}
	}
	return "", errors.New("nats file not found")

}

func NewNatsStreamingFactory() (*NatsStreamingFactory, error) {
	nats := os.Getenv("NATS_SERVER")
	tmpdir := ""
	if nats == "" {
		nats = "nats-streaming-server"
		_, err := exec.LookPath(nats)
		if err != nil {

			tmpdir, err = os.MkdirTemp(os.Getenv("HELPER_TMP"), "nats")
			if err != nil {
				return nil, err
			}
			nats, err = downloadNats(tmpdir)
			if err != nil {
				os.RemoveAll(tmpdir)
				return nil, err
			}
		}

	}

	return &NatsStreamingFactory{
		nats:   nats,
		tmpdir: tmpdir,
	}, nil
}

func (gf *NatsStreamingFactory) NewNatsStreamingInstance() (*NatsStreamingInstance, error) {

	tmpdir, err := os.MkdirTemp(os.Getenv("HELPER_TMP"), "nats")
	if err != nil {
		return nil, err
	}

	gi := &NatsStreamingInstance{
		nats:   gf.nats,
		tmpdir: tmpdir,
	}

	if err != nil {
		return nil, err
	}
	return gi, nil
}

func (gf *NatsStreamingFactory) Clean() error {
	if gf == nil {
		return nil
	}
	if gf.tmpdir != "" {
		os.RemoveAll(gf.tmpdir)

	}
	return nil
}

type NatsStreamingInstance struct {
	nats string

	tmpdir string
	cmd    *exec.Cmd
}

func (nsi *NatsStreamingInstance) NatsPort() uint32 {
	return 4222
}

func (nsi *NatsStreamingInstance) ClusterId() string {
	return "test-cluster"
}

func (nsi *NatsStreamingInstance) Run() error {

	var cmd *exec.Cmd
	natsargs := []string{"-DV"}
	cmd = exec.Command(nsi.nats, natsargs...)

	cmd.Dir = nsi.tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	err := cmd.Start()
	if err != nil {
		return err
	}
	time.Sleep(time.Second / 3)
	nsi.cmd = cmd
	return nil
}

func (nsi *NatsStreamingInstance) Clean() error {
	if nsi == nil {
		return nil
	}
	if nsi.cmd != nil {
		nsi.cmd.Process.Kill()
		nsi.cmd.Wait()
	}
	if nsi.tmpdir != "" {
		defer os.RemoveAll(nsi.tmpdir)

	}

	return nil
}
