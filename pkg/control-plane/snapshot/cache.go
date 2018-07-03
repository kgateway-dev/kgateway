package snapshot

import (
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/control-plane/filewatcher"
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

// no need to configure anything until some routes (virtual services) exist
// endpoints don't matter as far as envoy is concerned
func (c *Cache) Ready() bool {
	return c.Cfg != nil || len(c.Cfg.VirtualServices) < 1
}

func (c *Cache) Hash() uint64 {
	if !c.Ready() {
		return 0
	}
	cfgForHashing := proto.Clone(c.Cfg).(*v1.Config)
	for _, us := range cfgForHashing.Upstreams {
		us.Status = nil
		if us.Metadata != nil {
			us.Metadata.ResourceVersion = ""
		}
		us.Metadata = nil
	}
	for _, vs := range cfgForHashing.VirtualServices {
		vs.Status = nil
		if vs.Metadata != nil {
			vs.Metadata.ResourceVersion = ""
		}
		vs.Metadata = nil
	}
	for _, role := range cfgForHashing.Roles {
		role.Status = nil
		if role.Metadata != nil {
			role.Metadata.ResourceVersion = ""
		}
		role.Metadata = nil
	}
	for _, attribute := range cfgForHashing.Attributes {
		attribute.Status = nil
		if attribute.Metadata != nil {
			attribute.Metadata.ResourceVersion = ""
		}
		attribute.Metadata = nil
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
