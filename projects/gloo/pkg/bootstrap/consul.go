package bootstrap

// import (
// 	"context"

// 	"github.com/hashicorp/consul/api"
// 	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
// 	"github.com/solo-io/go-utils/contextutils"
// 	"github.com/solo-io/solo-kit/pkg/utils/prototime"
// )

// func ConsulClientForSettings(ctx context.Context, settings *v1.Settings) (*api.Client, error) {
// 	cfg := api.DefaultConfig()

// 	consulSettings := settings.GetConsul()
// 	if consulSettings != nil {
// 		if addr := consulSettings.GetAddress(); addr != "" {
// 			contextutils.LoggerFrom(ctx).Warnf("Consul `address` (%s) is deprecated in favor of `http_address`", addr)
// 			cfg.Address = addr
// 		}
// 		if addr := consulSettings.GetHttpAddress(); addr != "" {
// 			cfg.Address = addr
// 		}
// 		if dc := consulSettings.GetDatacenter(); dc != "" {
// 			cfg.Datacenter = dc
// 		}
// 		if user := consulSettings.GetUsername(); user != "" {
// 			if cfg.HttpAuth == nil {
// 				cfg.HttpAuth = &api.HttpBasicAuth{}
// 			}
// 			cfg.HttpAuth.Username = user
// 		}
// 		if pass := consulSettings.GetPassword(); pass != "" {
// 			if cfg.HttpAuth == nil {
// 				cfg.HttpAuth = &api.HttpBasicAuth{}
// 			}
// 			cfg.HttpAuth.Password = pass
// 		}
// 		if token := consulSettings.GetToken(); token != "" {
// 			cfg.Token = token
// 		}
// 		if caFile := consulSettings.GetCaFile(); caFile != "" {
// 			cfg.TLSConfig.CAFile = caFile
// 		}
// 		if caPath := consulSettings.GetCaPath(); caPath != "" {
// 			cfg.TLSConfig.CAPath = caPath
// 		}
// 		if certFile := consulSettings.GetCertFile(); certFile != "" {
// 			cfg.TLSConfig.CertFile = certFile
// 		}
// 		if keyFile := consulSettings.GetKeyFile(); keyFile != "" {
// 			cfg.TLSConfig.KeyFile = keyFile
// 		}
// 		if insecureSkipVerify := consulSettings.GetInsecureSkipVerify(); insecureSkipVerify != nil {
// 			cfg.TLSConfig.InsecureSkipVerify = insecureSkipVerify.GetValue()
// 		}
// 		if waitTime := consulSettings.GetWaitTime(); waitTime != nil {
// 			duration := prototime.DurationFromProto(waitTime)
// 			cfg.WaitTime = duration
// 		}
// 	}

// 	return api.NewClient(cfg)
// }
