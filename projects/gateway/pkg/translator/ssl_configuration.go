package translator

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func groupVirtualServicesBySslConfig(virtualServices []*v1.VirtualService) map[*gloov1.SslConfig][]*v1.VirtualService {
	result := map[*gloov1.SslConfig][]*v1.VirtualService{}
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

	for sslHash, sslConfig := range mergedSslConfig {
		result[sslConfig] = groupedVirtualServices[sslHash]
	}
	return result
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
