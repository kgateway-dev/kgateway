package translator

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func ConsolidateSslConfigurations(sslConfigurations []*ssl.SslConfig) []*ssl.SslConfig {
	var result []*ssl.SslConfig
	mergedSslSecrets := map[string]*ssl.SslConfig{}

	for _, sslConfig := range sslConfigurations {
		if sslConfig == nil {
			continue
		}
		// make sure ssl configs are only different by sni domains
		sslConfigCopy := proto.Clone(sslConfig).(*ssl.SslConfig)
		sslConfigCopy.SniDomains = nil

		hash, _ := sslConfigCopy.Hash(nil)
		key := fmt.Sprintf("%d", hash)

		if matchingCfg, ok := mergedSslSecrets[key]; ok {
			if len(matchingCfg.GetSniDomains()) == 0 || len(sslConfig.GetSniDomains()) == 0 {
				// if either of the configs match on everything; then match on everything
				matchingCfg.SniDomains = nil
			} else {
				matchingCfg.SniDomains = merge(matchingCfg.GetSniDomains(), sslConfig.GetSniDomains()...)
			}
		} else {
			ptrToCopy := proto.Clone(sslConfig).(*ssl.SslConfig)
			mergedSslSecrets[key] = ptrToCopy
			result = append(result, ptrToCopy)
		}
	}

	return result
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

// Inspired by: https://github.com/solo-io/gloo/blob/0ad2a02a816be2b4a8b6ce27ff9db01206ce6ceb/projects/gateway/pkg/translator/merge_options.go#L10

// Merges the fields of src into dst.
// The fields in dst that have non-zero values will not be overwritten.
func MergeSslConfig(dst, src *ssl.SslConfig) *ssl.SslConfig {
	if src == nil {
		return dst
	}

	if dst == nil {
		return proto.Clone(src).(*ssl.SslConfig)
	}

	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()

	for i := 0; i < dstValue.NumField(); i++ {
		dstField, srcField := dstValue.Field(i), srcValue.Field(i)
		utils.ShallowMerge(dstField, srcField, false)
	}

	return dst
}
