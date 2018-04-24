package file

import (
	"os"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/pkg/storage"
)

type Client struct {
	v1 *v1client
}

const upstreamsDir = "upstreams"
const virtualServicesDir = "virtualservices"

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
		},
	}, nil
}

func (c *Client) V1() storage.V1 {
	return c.v1
}

type v1client struct {
	upstreams       *upstreamsClient
	virtualServices *virtualServicesClient
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
	return nil
}

func (c *v1client) Upstreams() storage.Upstreams {
	return c.upstreams
}

func (c *v1client) VirtualServices() storage.VirtualServices {
	return c.virtualServices
}
