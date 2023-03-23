package translator

import (
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
)

// GroupVirtualServicesBySslConfig returning a stable order of sslConfigs
// and a map of sslconfigs to their associated Virtual service lists to use on.
func GroupVirtualServicesBySslConfig(virtualServices []*v1.VirtualService) ([]*ssl.SslConfig, map[*ssl.SslConfig][]*v1.VirtualService) {
	mergedSslConfig := map[string]*ssl.SslConfig{}
	groupedVirtualServices := map[string][]*v1.VirtualService{}

	for _, virtualService := range virtualServices {
		sslConfig := virtualService.GetSslConfig()
		sslConfigHash := hashSslConfig(sslConfig)

		if matchingCfg, ok := mergedSslConfig[sslConfigHash]; ok {
			// there is an existing sslConfig that differs only by sni domain, update the entry
			if len(matchingCfg.GetSniDomains()) == 0 || len(sslConfig.GetSniDomains()) == 0 {
				// if either of the configs match on everything; then match on everything
				matchingCfg.SniDomains = nil
			} else {
				matchingCfg.SniDomains = merge(matchingCfg.GetSniDomains(), sslConfig.GetSniDomains()...)
			}
			groupedVirtualServices[sslConfigHash] = append(groupedVirtualServices[sslConfigHash], virtualService)

		} else {
			// there is no existing sslConfig, create a new entry
			ptrToCopy := proto.Clone(sslConfig).(*ssl.SslConfig)
			mergedSslConfig[sslConfigHash] = ptrToCopy
			groupedVirtualServices[sslConfigHash] = []*v1.VirtualService{virtualService}
		}
	}

	// get an order of the strings as they are easier to compute once
	// rather than adding as the sort criterion for sslconfigs after the fact
	orderedHashes := make([]string, 0, len(mergedSslConfig))
	for sslHash := range mergedSslConfig {
		orderedHashes = append(orderedHashes, sslHash)
	}
	sort.Strings(orderedHashes)

	result := map[*ssl.SslConfig][]*v1.VirtualService{}
	orderedResultKeys := make([]*ssl.SslConfig, 0, len(mergedSslConfig))

	for _, sslHash := range orderedHashes {
		sslConfig := mergedSslConfig[sslHash]
		orderedResultKeys = append(orderedResultKeys, sslConfig)
		result[sslConfig] = groupedVirtualServices[sslHash]
	}
	return orderedResultKeys, result
}

func hashSslConfig(sslConfig *ssl.SslConfig) string {
	// make sure ssl configs are only different by sni domains
	sslConfigCopy := proto.Clone(sslConfig).(*ssl.SslConfig)
	sslConfigCopy.SniDomains = nil

	hash, _ := sslConfigCopy.Hash(nil)
	return fmt.Sprintf("%d", hash)
}

func merge(values []string, newValues ...string) []string {
	existingValues := make(map[string]struct{}, len(values))
	for _, v := range values {
		existingValues[v] = struct{}{}
	}

	for _, v := range newValues {
		if _, ok := existingValues[v]; !ok {
			values = append(values, v)
		}
	}
	return values
}
