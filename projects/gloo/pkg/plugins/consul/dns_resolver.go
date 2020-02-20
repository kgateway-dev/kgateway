package consul

import (
	"context"
	"net"

	"github.com/rotisserie/eris"
)

//go:generate mockgen -destination ./mocks/dnsresolver_mock.go github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul DnsResolver
//go:generate gofmt -w ./mocks/
//go:generate goimports -w ./mocks/

type DnsResolver interface {
	Resolve(address string) ([]net.IPAddr, error)
}

type ConsulDnsResolver struct {
	DnsAddress string
}

func (c *ConsulDnsResolver) Resolve(address string) ([]net.IPAddr, error) {
	res := net.Resolver{
		PreferGo: true, // otherwise we may use cgo which doesn't resolve on my mac in testing
		Dial: func(ctx context.Context, network, address string) (conn net.Conn, err error) {
			// DNS typically uses UDP and falls back to TCP if the response size is greater than one packet
			// (originally 512 bytes). we use TCP to ensure we receive all IPs in a large DNS response
			return net.Dial("tcp", c.DnsAddress)
		},
	}
	ipAddrs, err := res.LookupIPAddr(context.Background(), address)
	if err != nil {
		return nil, err
	}
	if len(ipAddrs) == 0 {
		return nil, eris.Errorf("Consul service returned an address that couldn't be parsed as an IP (%s), "+
			"resolved as a hostname at %s but the DNS server returned no results", address, c.DnsAddress)
	}
	return ipAddrs, nil
}
