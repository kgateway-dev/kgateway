package consul

import (
	"context"
	"net"
	"sync"

	"github.com/rotisserie/eris"
)

//go:generate mockgen -destination ./mocks/dnsresolver_mock.go github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul DnsResolver

type DnsResolver interface {
	Resolve(ctx context.Context, address string) ([]net.IPAddr, error)
}

var (
	_ DnsResolver = new(dnsResolver)
	_ DnsResolver = new(dnsResolverWithFallback)
)

func NewConsulDnsResolver(address string) DnsResolver {
	basicResolver := &dnsResolver{
		DnsAddress: address,
	}
	return NewDnsResolverWithFallback(basicResolver)
}

type dnsResolver struct {
	DnsAddress string
}

func (c *dnsResolver) Resolve(ctx context.Context, address string) ([]net.IPAddr, error) {
	res := net.Resolver{
		PreferGo: true, // otherwise we may use cgo which doesn't resolve on my mac in testing
		Dial: func(ctx context.Context, network, address string) (conn net.Conn, err error) {
			// DNS typically uses UDP and falls back to TCP if the response size is greater than one packet
			// (originally 512 bytes). we use TCP to ensure we receive all IPs in a large DNS response
			return net.Dial("tcp", c.DnsAddress)
		},
	}
	ipAddrs, err := res.LookupIPAddr(ctx, address)
	if err != nil {
		return nil, err
	}
	if len(ipAddrs) == 0 {
		return nil, eris.Errorf("Consul service returned an address that couldn't be parsed as an IP (%s), "+
			"resolved as a hostname at %s but the DNS server returned no results", address, c.DnsAddress)
	}
	return ipAddrs, nil
}

type dnsResolverWithFallback struct {
	resolver DnsResolver

	sync.RWMutex
	previousResolutions map[string][]net.IPAddr
}

func NewDnsResolverWithFallback(resolver DnsResolver) *dnsResolverWithFallback {
	return &dnsResolverWithFallback{
		resolver:            resolver,
		previousResolutions: make(map[string][]net.IPAddr),
	}
}

func (d *dnsResolverWithFallback) Resolve(ctx context.Context, address string) ([]net.IPAddr, error) {
	ipAddrs, err := d.resolver.Resolve(ctx, address)

	// Synchronize access to previous resolutions
	d.Lock()
	defer d.Unlock()

	// If we successfully resolved the addresses, update our last known state and return
	if err == nil {
		d.previousResolutions[address] = ipAddrs
		return ipAddrs, nil
	}

	// If we did not successfully resolve the addresses, attempt to use the last known state
	lastKnownIdAddrs, resolvedPreviously := d.previousResolutions[address]
	if !resolvedPreviously {
		return nil, err
	}
	return lastKnownIdAddrs, nil
}
