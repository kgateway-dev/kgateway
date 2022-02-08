package translator

import (
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type HttpGatewaySelector interface {
	SelectMatchableHttpGateways(selector *v1.DelegatedHttpGateway, onError func(err error)) v1.MatchableHttpGatewayList
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

type gatewaySelector struct {
	availableGateways v1.MatchableHttpGatewayList
}

func NewHttpGatewaySelector(gwList v1.MatchableHttpGatewayList) *gatewaySelector {
	return &gatewaySelector{
		availableGateways: gwList,
	}
}

func (s *gatewaySelector) SelectMatchableHttpGateways(selector *v1.DelegatedHttpGateway, onError func(err error)) v1.MatchableHttpGatewayList {
	var selectedGateways v1.MatchableHttpGatewayList

	s.availableGateways.Each(func(element *v1.MatchableHttpGateway) {
		selected, err := s.isSelected(element, selector)
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

func (s *gatewaySelector) isSelected(matchableHttpGateway *v1.MatchableHttpGateway, selector *v1.DelegatedHttpGateway) (bool, error) {
	if selector == nil {
		return false, nil
	}

	refSelector := selector.GetRef()
	if refSelector != nil {
		return refSelector.Equal(matchableHttpGateway.GetMetadata().Ref()), nil
	}

	gwLabels := labels.Set(matchableHttpGateway.GetMetadata().GetLabels())
	gwNamespace := matchableHttpGateway.GetMetadata().GetNamespace()

	doesMatchNamespaces := matchNamespaces(gwNamespace, selector.GetSelector().GetNamespaces())
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
