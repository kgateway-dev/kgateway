package validation

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func ValidateUpstream(apiSnapshot *v1.ApiSnapshot, upstream *v1.Upstream) *validation.UpstreamReport {
	var errors []*validation.UpstreamReport_Error

	if err := ValidateSslConfig(apiSnapshot, upstream); err != nil {
		errors = append(errors, err)
	}

	return &validation.UpstreamReport{Errors: errors}
}

func ValidateSslConfig(apiSnapshot *v1.ApiSnapshot, upstream *v1.Upstream) *validation.UpstreamReport_Error {
	if secretRef := upstream.GetSslConfig().GetSecretRef(); secretRef != nil {
		if _, err := apiSnapshot.Secrets.Find(secretRef.GetNamespace(), secretRef.GetName()); err != nil {
			return &validation.UpstreamReport_Error{
				Type:   validation.UpstreamReport_Error_SSL_CONFIG_ERROR,
				Reason: "Secret does not exist",
			}
		}
	}
	return nil
}
