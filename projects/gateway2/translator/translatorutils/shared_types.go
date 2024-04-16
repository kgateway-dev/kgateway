package translatorutils

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
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

func GetProxyId(proxy *gloov1.Proxy) (string, error) {
	labels := proxy.GetMetadata().GetLabels()
	if labels == nil {
		return "", eris.Errorf("proxy %v missing labels", proxy.GetMetadata().Ref())
	}
	proxyId, ok := labels[utils.ProxyId]
	if !ok {
		return "", eris.Errorf("proxy %v missing label %v", proxy.GetMetadata().Ref(), utils.ProxyId)
	}
	return proxyId, nil
}
