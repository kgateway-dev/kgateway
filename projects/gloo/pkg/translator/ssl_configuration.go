package translator

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
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
// Copied from the other project to reduce dependency.

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
		shallowMerge(dstField, srcField, false)
	}

	return dst
}

// Sets dst to the value of src, if src is non-zero and dest is zero-valued or overwrite=true.
func shallowMerge(dst, src reflect.Value, overwrite bool) {
	if !src.IsValid() {
		return
	}

	if dst.CanSet() && !isEmptyValue(src) && (overwrite || isEmptyValue(dst)) {
		dst.Set(src)
	}

	return
}

// From src/pkg/encoding/json/encode.go.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return true
		}
		return isEmptyValue(v.Elem())
	case reflect.Func:
		return v.IsNil()
	case reflect.Invalid:
		return true
	}
	return false
}
