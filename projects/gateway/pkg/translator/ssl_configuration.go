package translator

import (
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// groupVirtualServicesBySslConfig returning a stable order of sslConfigs
// and a map of sslconfigs to their associated Virtual service lists to use on.
func groupVirtualServicesBySslConfig(virtualServices []*v1.VirtualService) ([]*gloov1.SslConfig, map[*gloov1.SslConfig][]*v1.VirtualService) {
	mergedSslConfig := map[string]*gloov1.SslConfig{}
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
			groupedVirtualServices[sslConfigHash] = []*v1.VirtualService{virtualService}

		} else {
			// there is no existing sslConfig, create a new entry
			ptrToCopy := proto.Clone(sslConfig).(*gloov1.SslConfig)
			mergedSslConfig[sslConfigHash] = ptrToCopy
			groupedVirtualServices[sslConfigHash] = append(groupedVirtualServices[sslConfigHash], virtualService)
		}
	}

	// get an order of the strings as they are easier to compute once
	// rather than adding as the sort criterion for sslconfigs after the fact
	orderedHashes := make([]string, 0, len(mergedSslConfig))
	for sslHash := range mergedSslConfig {
		orderedHashes = append(orderedHashes, sslHash)
	}
	sort.Strings(orderedHashes)

	result := map[*gloov1.SslConfig][]*v1.VirtualService{}
	orderedResultKeys := make([]*gloov1.SslConfig, 0, len(mergedSslConfig))

	for _, sslHash := range orderedHashes {
		sslConfig := mergedSslConfig[sslHash]
		orderedResultKeys = append(orderedResultKeys, sslConfig)
		result[sslConfig] = groupedVirtualServices[sslHash]
	}
	return orderedResultKeys, result
}

func hashSslConfig(sslConfig *gloov1.SslConfig) string {
	// make sure ssl configs are only different by sni domains
	sslConfigCopy := proto.Clone(sslConfig).(*gloov1.SslConfig)
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
