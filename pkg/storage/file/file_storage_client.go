package file

import (
	"os"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/pkg/storage"
)

//go:generate go run ${GOPATH}/src/github.com/solo-io/gloo/pkg/storage/generate/generate_clients.go -f ${GOPATH}/src/github.com/solo-io/gloo/pkg/storage/file/client_template.go.tmpl -o ${GOPATH}/src/github.com/solo-io/gloo/pkg/storage/file/

type Client struct {
	v1 *v1client
}

const (
	upstreamsDir       = "upstreams"
	virtualServicesDir = "virtualservices"
	reportsDir         = "reports"
)

func NewStorage(dir string, syncFrequency time.Duration) (storage.Interface, error) {
	if dir == "" {
		dir = GlooDefaultDirectory
	}
	return &Client{
		v1: &v1client{
			upstreams: &upstreamsClient{
				dir:           filepath.Join(dir, upstreamsDir),
				syncFrequency: syncFrequency,
			},
			virtualServices: &virtualServicesClient{
				dir:           filepath.Join(dir, virtualServicesDir),
				syncFrequency: syncFrequency,
			},
			reports: &reportsClient{
				dir:           filepath.Join(dir, reportsDir),
				syncFrequency: syncFrequency,
			},
		},
	}, nil
}

func (c *Client) V1() storage.V1 {
	return c.v1
}

type v1client struct {
	upstreams       *upstreamsClient
	virtualServices *virtualServicesClient
	reports         *reportsClient
}

func (c *v1client) Register() error {
	err := os.MkdirAll(c.upstreams.dir, 0755)
	if err != nil && err != os.ErrExist {
		return err
	}
	err = os.MkdirAll(c.virtualServices.dir, 0755)
	if err != nil && err != os.ErrExist {
		return err
	}
	err = os.MkdirAll(c.reports.dir, 0755)
	if err != nil && err != os.ErrExist {
		return err
	}
	return nil
}

func (c *v1client) Upstreams() storage.Upstreams {
	return c.upstreams
}

func (c *v1client) VirtualServices() storage.VirtualServices {
	return c.virtualServices
}

func (c *v1client) Reports() storage.Reports {
	return c.reports
}
