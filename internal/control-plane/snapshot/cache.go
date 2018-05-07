package snapshot

import (
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/internal/control-plane/filewatcher"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/mitchellh/hashstructure"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/gogo/protobuf/proto"
)

// Cache contains the latest Gloo snapshot
type Cache struct {
	Cfg       *v1.Config
	Secrets   secretwatcher.SecretMap
	Files     filewatcher.Files
	Endpoints endpointdiscovery.EndpointGroups
}

func newCache() *Cache {
	return &Cache{}
}

// ready doesn't necessarily tell us whetehr endpoints have been discovered yet
// but that's okay. envoy won't mind
func (c *Cache) Ready() bool {
	return c.Cfg != nil
}

func (c *Cache) Hash() uint64 {
	if !c.Ready() {
		return 0
	}
	cfgForHashing := proto.Clone(c.Cfg).(*v1.Config)
	for _, us := range cfgForHashing.Upstreams {
		if us.Metadata != nil {
			us.Metadata.ResourceVersion = ""
		}
		us.Metadata = nil
	}
	for _, vs := range cfgForHashing.VirtualServices {
		if vs.Metadata != nil {
			vs.Metadata.ResourceVersion = ""
		}
		vs.Metadata = nil
	}
	h0, err := hashstructure.Hash(*cfgForHashing, nil)
	if err != nil {
		panic(err)
	}
	h1, err := hashstructure.Hash(c.Secrets, nil)
	if err != nil {
		panic(err)
	}
	h2, err := hashstructure.Hash(c.Endpoints, nil)
	if err != nil {
		panic(err)
	}
	h3, err := hashstructure.Hash(c.Files, nil)
	if err != nil {
		panic(err)
	}
	h := h0 + h1 + h2 + h3
	return h
}
