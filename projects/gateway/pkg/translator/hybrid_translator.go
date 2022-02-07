package translator

import (
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ ListenerTranslator = new(HybridTranslator)

const HybridTranslatorName = "hybrid"

var (
	EmptyHybridGatewayErr = func() error {
		return errors.Errorf("hybrid gateway does not have any populated matched gateways")
	}
)

type HybridTranslator struct {
	HttpTranslator *HttpTranslator
	TcpTranslator  *TcpTranslator
}

func (t *HybridTranslator) Name() string {
	return HybridTranslatorName
}

func (t *HybridTranslator) ComputeListener(params Params, proxyName string, gateway *v1.Gateway) *gloov1.Listener {
	hybridGateway := gateway.GetHybridGateway()
	if hybridGateway == nil {
		return nil
	}

	var hybridListener *gloov1.HybridListener

	// MatchedGateways take precedence
	matchedGateways := hybridGateway.GetMatchedGateways()
	if len(matchedGateways) > 0 {
		hybridListener = t.ComputeHybridListenerFromMatchedGateways(params, proxyName, gateway, matchedGateways)
	}

	// DelegatedHttpGateways is only processed if there are no MatchedGateways defined
	if hybridListener == nil {
		hybridListener = t.ComputeHybridListenerFromDelegatedGateways(params, proxyName, gateway, hybridGateway.GetDelegatedHttpGateways())
	}

	if len(hybridListener.GetMatchedListeners()) == 0 {
		params.reports.AddError(gateway, EmptyHybridGatewayErr())
		return nil
	}

	listener := makeListener(gateway)
	listener.ListenerType = &gloov1.Listener_HybridListener{
		HybridListener: hybridListener,
	}

	if err := appendSource(listener, gateway); err != nil {
		// should never happen
		params.reports.AddError(gateway, err)
	}

	return listener
}

func (t *HybridTranslator) ComputeHybridListenerFromMatchedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	matchedGateways []*v1.MatchedGateway,
) *gloov1.HybridListener {
	snap := params.snapshot
	hybridListener := &gloov1.HybridListener{}
	loggedError := false

	for _, matchedGateway := range matchedGateways {
		// Initialize the MatchedListener
		matchedListener := &gloov1.MatchedListener{
			Matcher: &gloov1.Matcher{
				SslConfig:          matchedGateway.GetMatcher().GetSslConfig(),
				SourcePrefixRanges: matchedGateway.GetMatcher().GetSourcePrefixRanges(),
			},
		}

		switch gt := matchedGateway.GetGatewayType().(type) {
		case *v1.MatchedGateway_HttpGateway:
			if len(snap.VirtualServices) == 0 {
				if !loggedError {
					snapHash := hashutils.MustHash(snap)
					contextutils.LoggerFrom(params.ctx).Debugf("%v had no virtual services", snapHash)
					loggedError = true // only log no virtual service error once
				}
				continue
			}

			httpGateway := matchedGateway.GetHttpGateway()
			sslGateway := matchedGateway.GetMatcher().GetSslConfig() != nil
			virtualServices := getVirtualServicesForHttpGateway(params, gateway, httpGateway, sslGateway)

			matchedListener.ListenerType = &gloov1.MatchedListener_HttpListener{
				HttpListener: t.HttpTranslator.ComputeHttpListener(params, gateway, httpGateway, virtualServices, proxyName),
			}

			if sslGateway {
				virtualServices.Each(func(vs *v1.VirtualService) {
					matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
				})
			}

		case *v1.MatchedGateway_TcpGateway:
			matchedListener.ListenerType = &gloov1.MatchedListener_TcpListener{
				TcpListener: t.TcpTranslator.ComputeTcpListener(gt.TcpGateway),
			}
		}

		hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
	}

	return hybridListener
}

func (t *HybridTranslator) ComputeHybridListenerFromDelegatedGateways(
	params Params,
	proxyName string,
	gateway *v1.Gateway,
	delegatedGateway *v1.DelegatedHttpGateway,
) *gloov1.HybridListener {
	gatewaySelector := newHttpGatewaySelector(params.snapshot)
	onError := func(err error) {
		params.reports.AddError(gateway, err)
	}
	matchableHttpGateways := gatewaySelector.SelectMatchableHttpGateways(delegatedGateway, onError)
	if len(matchableHttpGateways) == 0 {
		return nil
	}

	hybridListener := &gloov1.HybridListener{}

	matchableHttpGateways.Each(func(element *v1.MatchableHttpGateway) {
		matchedListener := t.computeMatchedListener(params, proxyName, gateway, element)
		if matchedListener != nil {
			hybridListener.MatchedListeners = append(hybridListener.GetMatchedListeners(), matchedListener)
		}
	})

	return hybridListener
}

func (t *HybridTranslator) computeMatchedListener(
	params Params,
	proxyName string,
	parentGateway *v1.Gateway,
	matchableHttpGateway *v1.MatchableHttpGateway,
) *gloov1.MatchedListener {
	matchedListener := &gloov1.MatchedListener{
		Matcher: &gloov1.Matcher{
			SslConfig:          matchableHttpGateway.GetMatcher().GetSslConfig(),
			SourcePrefixRanges: matchableHttpGateway.GetMatcher().GetSourcePrefixRanges(),
		},
	}

	httpGateway := matchableHttpGateway.GetHttpGateway()
	sslGateway := matchableHttpGateway.GetMatcher().GetSslConfig() != nil
	virtualServices := getVirtualServicesForHttpGateway(params, parentGateway, httpGateway, sslGateway)

	matchedListener.ListenerType = &gloov1.MatchedListener_HttpListener{
		HttpListener: t.HttpTranslator.ComputeHttpListener(params, parentGateway, httpGateway, virtualServices, proxyName),
	}

	if sslGateway {
		virtualServices.Each(func(vs *v1.VirtualService) {
			matchedListener.SslConfigurations = append(matchedListener.GetSslConfigurations(), vs.GetSslConfig())
		})
	}

	return matchedListener
}

var (
	SelectorInvalidExpressionWarning = errors.New("the http gateway selector expression is invalid")
	SelectorExpressionOperatorValues = map[selectors.Selector_Expression_Operator]selection.Operator{
		selectors.Selector_Expression_Equals:       selection.Equals,
		selectors.Selector_Expression_DoubleEquals: selection.DoubleEquals,
		selectors.Selector_Expression_NotEquals:    selection.NotEquals,
		selectors.Selector_Expression_In:           selection.In,
		selectors.Selector_Expression_NotIn:        selection.NotIn,
		selectors.Selector_Expression_Exists:       selection.Exists,
		selectors.Selector_Expression_DoesNotExist: selection.DoesNotExist,
		selectors.Selector_Expression_GreaterThan:  selection.GreaterThan,
		selectors.Selector_Expression_LessThan:     selection.LessThan,
	}
)

type httpGatewaySelector struct {
	availableGateways v1.MatchableHttpGatewayList
}

func newHttpGatewaySelector(snapshot *v1.ApiSnapshot) *httpGatewaySelector {
	return &httpGatewaySelector{
		availableGateways: snapshot.HttpGateways,
	}
}

func (s *httpGatewaySelector) SelectMatchableHttpGateways(selector *v1.DelegatedHttpGateway, onError func(err error)) v1.MatchableHttpGatewayList {
	var selectedGateways v1.MatchableHttpGatewayList

	for _, matchableHttpGateway := range s.availableGateways {
		selected, err := s.isSelected(matchableHttpGateway, selector)
		if err != nil {
			onError(err)
			continue
		}

		if selected {
			selectedGateways = append(selectedGateways, matchableHttpGateway)
		}
	}

	return selectedGateways
}

func (s *httpGatewaySelector) isSelected(matchableHttpGateway *v1.MatchableHttpGateway, selector *v1.DelegatedHttpGateway) (bool, error) {
	if selector == nil {
		return false, nil
	}

	refSelector := selector.GetRef()
	if refSelector != nil {
		return matchableHttpGateway.GetMetadata().Ref().Equal(refSelector), nil
	}

	gwLabels := labels.Set(matchableHttpGateway.GetMetadata().GetLabels())

	doesMatchNamespaces := matchNamespaces(matchableHttpGateway.GetMetadata().GetNamespace(), selector.GetSelector().GetNamespaces())
	doesMatchLabels := matchLabels(gwLabels, selector.GetSelector().GetLabels())
	doesMatchExpressions, err := matchExpressions(gwLabels, selector.GetSelector().GetExpressions())
	if err != nil {
		return false, err
	}

	return doesMatchNamespaces && doesMatchLabels && doesMatchExpressions, nil
}

func matchNamespaces(gatewayNs string, namespaces []string) bool {
	if len(namespaces) == 0 {
		return true
	}

	for _, ns := range namespaces {
		if ns == "*" || gatewayNs == ns {
			return true
		}
	}

	return false
}

func matchLabels(gatewayLabelSet labels.Set, validLabels map[string]string) bool {
	var labelSelector labels.Selector

	// Check whether labels match (strict equality)
	labelSelector = labels.SelectorFromSet(validLabels)
	return labelSelector.Matches(gatewayLabelSet)
}

func matchExpressions(gatewayLabelSet labels.Set, expressions []*selectors.Selector_Expression) (bool, error) {
	if expressions == nil {
		return true, nil
	}

	var requirements labels.Requirements
	for _, expression := range expressions {
		r, err := labels.NewRequirement(
			expression.GetKey(),
			SelectorExpressionOperatorValues[expression.GetOperator()],
			expression.GetValues())
		if err != nil {
			return false, errors.Wrap(SelectorInvalidExpressionWarning, err.Error())
		}
		requirements = append(requirements, *r)
	}

	return labelsMatchExpressionRequirements(requirements, gatewayLabelSet), nil
}

func labelsMatchExpressionRequirements(requirements labels.Requirements, labels labels.Set) bool {
	for _, r := range requirements {
		if !r.Matches(labels) {
			return false
		}
	}
	return true
}
