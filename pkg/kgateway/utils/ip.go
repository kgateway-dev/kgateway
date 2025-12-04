package utils

import (
	"fmt"
	"net"
)

// IsIpv4Address returns whether
// the provided address is valid IPv4, is pure(unmapped) IPv4, and if there was an error in the bindaddr
// This is used to distinguish between IPv4 and IPv6 addresses
func IsIpv4Address(bindAddress string) (validIpv4, strictIPv4 bool, err error) {
	bindIP := net.ParseIP(bindAddress)
	if bindIP == nil {
		// If bindAddress is not a valid textual representation of an IP address
		return false, false, fmt.Errorf("bindAddress %s is not a valid IP address", bindAddress)
	} else if bindIP.To4() == nil {
		// If bindIP is not an IPv4 address, To4 returns nil.
		// so this is not an acceptable ipv4
		return false, false, nil
	}
	return true, isPureIPv4Address(bindAddress), nil
}

// isPureIPv4Address checks the string to see if it is
// ipv4 and not ipv4 mapped into ipv6 space and not ipv6.
// Used as the standard net.Parse smashes everything to ipv6.
// Basically false if ::ffff:0.0.0.0 and true if 0.0.0.0
func isPureIPv4Address(ipString string) bool {
	for i := range len(ipString) {
		switch ipString[i] {
		case '.':
			return true
		case ':':
			return false
		}
	}
	return false
}
