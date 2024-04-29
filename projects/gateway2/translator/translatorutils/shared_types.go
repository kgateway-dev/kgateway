package translatorutils

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type TranslationReports struct {
	ProxyReport     *validation.ProxyReport
	ResourceReports reporter.ResourceReports
}

type ProxyWithReports struct {
	Proxy   *gloov1.Proxy
	Reports TranslationReports
}
