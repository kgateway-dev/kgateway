package translator

import (
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type HttpGatewaySelector interface {
	SelectMatchableHttpGateways(selector *v1.DelegatedHttpGateway, onError func(err error)) v1.MatchableHttpGatewayList
}

type TcpGatewaySelector interface {
	SelectMatchableTcpGateways(selector *v1.DelegatedTcpGateway, onError func(err error)) v1.MatchableTcpGatewayList
}

var (
	SelectorInvalidExpressionWarning = errors.New("the selector expression is invalid")
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

type tcpGatewaySelector struct {
	availableGateways v1.MatchableTcpGatewayList
}

func NewHttpGatewaySelector(gwList v1.MatchableHttpGatewayList) *httpGatewaySelector {
	return &httpGatewaySelector{
		availableGateways: gwList,
	}
}

func (s *httpGatewaySelector) SelectMatchableHttpGateways(selector *v1.DelegatedHttpGateway, onError func(err error)) v1.MatchableHttpGatewayList {
	var selectedGateways v1.MatchableHttpGatewayList

	s.availableGateways.Each(func(element *v1.MatchableHttpGateway) {
		selected, err := isHttpGatewaySelected(element, selector)
		if err != nil {
			onError(err)
			return
		}

		if selected {
			selectedGateways = append(selectedGateways, element)
		}
	})

	return selectedGateways
}

func isHttpGatewaySelected(matchableHttpGateway *v1.MatchableHttpGateway, selector *v1.DelegatedHttpGateway) (bool, error) {
	if selector == nil {
		return false, nil
	}

	selectorDefinesSSL := selector.GetSslConfig() != nil
	gwDefinesSSL := matchableHttpGateway.GetMatcher().GetSslConfig() != nil
	if selectorDefinesSSL != gwDefinesSSL {
		return false, nil
	}

	return matchMetadata(matchableHttpGateway.GetMetadata(), selector.GetRef(), selector.GetSelector())
}

func NewTcpGatewaySelector(gwList v1.MatchableTcpGatewayList) *tcpGatewaySelector {
	return &tcpGatewaySelector{
		availableGateways: gwList,
	}
}

func (s *tcpGatewaySelector) SelectMatchableTcpGateways(selector *v1.DelegatedTcpGateway, onError func(err error)) v1.MatchableTcpGatewayList {
	var selectedGateways v1.MatchableTcpGatewayList

	s.availableGateways.Each(func(element *v1.MatchableTcpGateway) {
		selected, err := isTcpGatewaySelected(element, selector)
		if err != nil {
			onError(err)
			return
		}

		if selected {
			selectedGateways = append(selectedGateways, element)
		}
	})

	return selectedGateways
}

func isTcpGatewaySelected(matchableTcpGateway *v1.MatchableTcpGateway, selector *v1.DelegatedTcpGateway) (bool, error) {
	if selector == nil {
		return false, nil
	}

	return matchMetadata(matchableTcpGateway.GetMetadata(), selector.GetRef(), selector.GetSelector())
}

func matchMetadata(meta *core.Metadata, refSelector *core.ResourceRef, selector *selectors.Selector) (bool, error) {
	if refSelector != nil {
		return refSelector.Equal(meta.Ref()), nil
	}

	gwLabels := labels.Set(meta.GetLabels())
	gwNamespace := meta.GetNamespace()

	doesMatchNamespaces := matchNamespaces(gwNamespace, selector.GetNamespaces())
	doesMatchLabels := matchLabels(gwLabels, selector.GetLabels())
	doesMatchExpressions, err := matchExpressions(gwLabels, selector.GetExpressions())
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
	// Check whether labels match (strict equality)
	labelSelector := labels.SelectorFromSet(validLabels)
	return labelSelector.Matches(gatewayLabelSet)
}

func matchExpressions(gatewayLabelSet labels.Set, expressions []*selectors.Selector_Expression) (bool, error) {
	if expressions == nil {
		return true, nil
	}

	var requirements labels.Requirements
	for _, expression := range expressions {
		operator := SelectorExpressionOperatorValues[expression.GetOperator()]
		r, err := labels.NewRequirement(expression.GetKey(), operator, expression.GetValues())
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
