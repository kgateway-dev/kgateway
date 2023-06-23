package helpers

import (
	"fmt"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
)

// upstreamBuilder contains options for building Upstreams to be included in scaled Snapshots
type upstreamBuilder struct {
	sniPattern sniPattern
}

type sniPattern int

const (
	noSni sniPattern = iota
	uniqueSni
	consistentSni
)

func NewUpstreamBuilder() *upstreamBuilder {
	return &upstreamBuilder{}
}

func (b *upstreamBuilder) WithUniqueSni() *upstreamBuilder {
	b.sniPattern = uniqueSni
	return b
}

func (b *upstreamBuilder) WithConsistentSni() *upstreamBuilder {
	b.sniPattern = consistentSni
	return b
}

func (b *upstreamBuilder) Build(i int) *v1.Upstream {
	up := Upstream(i)

	switch b.sniPattern {
	case uniqueSni:
		up.SslConfig = &ssl.UpstreamSslConfig{
			Sni: fmt.Sprintf("unique-domain-%d", i),
		}
	case consistentSni:
		up.SslConfig = &ssl.UpstreamSslConfig{
			Sni: "consistent-domain",
		}
	}

	return up
}
