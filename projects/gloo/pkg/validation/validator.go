package validation

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

const GlooGroup = "gloo.solo.io"

// GlooValidator is used to validate solo.io.gloo resources
type GlooValidator interface {
	Validate(ctx context.Context, proxy *gloov1.Proxy, snapshot *gloosnapshot.ApiSnapshot, delete bool) []*GlooValidationReport
}

type GlooValidatorConfig struct {
	Translator   gloo_translator.Translator
	XdsSanitizer sanitizer.XdsSanitizer
}

// NewGlooValidator will create a new GlooValidator
func NewGlooValidator(config GlooValidatorConfig) GlooValidator {
	return glooValidator{
		translator:   config.Translator,
		xdsSanitizer: config.XdsSanitizer,
	}
}

type glooValidator struct {
	translator   gloo_translator.Translator
	xdsSanitizer sanitizer.XdsSanitizer
}

type GlooValidationReport struct {
	Proxy           *gloov1.Proxy
	ProxyReport     *validation.ProxyReport
	ResourceReports reporter.ResourceReports
}

func (gv glooValidator) Validate(ctx context.Context, proxy *gloov1.Proxy, snapshot *gloosnapshot.ApiSnapshot, delete bool) []*GlooValidationReport {
	ctx = contextutils.WithLogger(ctx, "proxy-validator")

	var validationReports []*GlooValidationReport
	var proxiesToValidate gloov1.ProxyList

	if proxy != nil {
		proxiesToValidate = gloov1.ProxyList{proxy}
	} else {
		// if no proxy was passed in, call translate for all proxies in snapshot
		proxiesToValidate = snapshot.Proxies
	}

	if len(proxiesToValidate) == 0 {
		// This can occur when a Gloo resource (Upstream), is modified before the ApiSnapshot
		// contains any Proxies. Orphaned resources are never invalid, but they may be accepted
		// even if they are semantically incorrect.
		// This log line is attempting to identify these situations
		contextutils.LoggerFrom(ctx).Warnf("found no proxies to validate, accepting update without translating Gloo resources")
		return validationReports
	}

	params := plugins.Params{
		Ctx:      ctx,
		Snapshot: snapshot,
	}
	// Validation with gateway occurs in /projects/gateway/pkg/validation/validator.go, where validation for the Gloo
	// resources occurs in the following for loop.
	for _, proxy := range proxiesToValidate {
		xdsSnapshot, resourceReports, proxyReport := gv.translator.Translate(params, proxy)

		// Sanitize routes before sending report to gateway
		gv.xdsSanitizer.SanitizeSnapshot(ctx, snapshot, xdsSnapshot, resourceReports)
		routeErrorToWarnings(resourceReports, proxyReport)

		validationReports = append(validationReports, &GlooValidationReport{
			Proxy:           proxy,
			ProxyReport:     proxyReport,
			ResourceReports: resourceReports,
		})
	}

	return validationReports
}
