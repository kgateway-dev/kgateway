package matchers

import (
	"fmt"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"github.com/rotisserie/eris"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// SoloKitStatus defines the set of properties that we can validate from a core.Status
type SoloKitStatus struct {
	State               *core.Status_State
	Reason              string
	ReportedBy          string
	SubresourceStatuses map[string]SoloKitSubresourceStatus
	// TODO: implement as needed
	// Details             *structpb.Struct
	// Messages            []string
	// Custom is a generic matcher that can be applied to validate any other properties of a core.Status
	// Optional: If not provided, does not perform additional validation
	Custom types.GomegaMatcher
}

// SoloKitSubresourceStatus is a struct for subresource status fields
type SoloKitSubresourceStatus struct {
	Reason     string
	ReportedBy string
	State      string
}

// SoloKitNamespacedStatuses defines the set of properties that we can validate from a core.NamespacedStatuses
type SoloKitNamespacedStatuses struct {
	Statuses map[string]*SoloKitStatus
}

func HaveState(state core.Status_State) types.GomegaMatcher {
	return HaveStatus(&SoloKitStatus{
		State: &state,
	})
}

func HaveReportedBy(reporter string) types.GomegaMatcher {
	return HaveStatus(&SoloKitStatus{
		ReportedBy: reporter,
	})
}

func HaveSubResourceStatusState(subResourceName string, subResourceStatus SoloKitSubresourceStatus) types.GomegaMatcher {
	return HaveStatus(&SoloKitStatus{
		SubresourceStatuses: map[string]SoloKitSubresourceStatus{
			subResourceName: subResourceStatus,
		},
	})
}

func HaveAcceptedState() types.GomegaMatcher {
	st := core.Status_Accepted
	return HaveStatus(&SoloKitStatus{
		State: &st,
	})
}

func HaveWarningStateWithReasonSubstrings(reasons ...string) types.GomegaMatcher {
	m := HaveReasonSubstrings(reasons...)
	st := core.Status_Warning
	m.(*HaveStatusMatcher).Expected.State = &st
	return m
}

func HaveRejectedStateWithReasonSubstrings(reasons ...string) types.GomegaMatcher {
	m := HaveReasonSubstrings(reasons...)
	st := core.Status_Rejected
	m.(*HaveStatusMatcher).Expected.State = &st
	return m
}

func HaveReasonSubstrings(reasons ...string) types.GomegaMatcher {
	return HaveStatus(&SoloKitStatus{
		Custom: gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Reason": ContainSubstrings(reasons),
		}),
	})
}

// MatchStatusInNamespace will create a matcher that allows a *HaveStatusMatcher generated from this
// package to be matched against the provided namespace in a HaveNamespacedStatusesMatcher
func MatchStatusInNamespace(ns string, matcher types.GomegaMatcher) types.GomegaMatcher {
	// TODO the dev ux of this isn't great since we will not have data for matchers of different types,
	// though we should expect not to receive such matchers.
	expected := &SoloKitStatus{}
	m, ok := matcher.(*HaveStatusMatcher)
	if ok {
		expected = m.Expected
	}

	return &HaveNamespacedStatusesMatcher{
		Expected: &SoloKitNamespacedStatuses{
			Statuses: map[string]*SoloKitStatus{
				ns: expected,
			},
		},
		namespacedStatusesMatchers: map[string]types.GomegaMatcher{
			ns: matcher,
		},
		evaluated: false,
	}
}

func HaveStatusInNamespace(ns string, status *core.Status) types.GomegaMatcher {
	st := status.GetState()
	return HaveNamespacedStatuses(&SoloKitNamespacedStatuses{
		Statuses: map[string]*SoloKitStatus{
			ns: {
				State:      &st,
				Reason:     status.GetReason(),
				ReportedBy: status.GetReportedBy(),
			},
		},
	})
}

func HaveNamespacedStatuses(expected *SoloKitNamespacedStatuses) types.GomegaMatcher {
	if expected == nil {
		// If no status is defined, we create a matcher that always succeeds
		return gstruct.Ignore()
	}
	namespacedStatusMatchers := map[string]types.GomegaMatcher{}
	for ns, expectedStatus := range expected.Statuses {
		namespacedStatusMatchers[ns] = HaveStatus(expectedStatus)
	}
	return &HaveNamespacedStatusesMatcher{
		Expected:                   expected,
		namespacedStatusesMatchers: namespacedStatusMatchers,
		evaluated:                  false,
	}
}

// HaveStatus produces a matcher that will match if the provided status matches the
// actual status
func HaveStatus(expected *SoloKitStatus) types.GomegaMatcher {
	if expected == nil {
		// If no status is defined, we create a matcher that always succeeds
		return gstruct.Ignore()
	}
	expectedCustomMatcher := expected.Custom
	if expectedCustomMatcher == nil {
		// Default to an always accept matcher
		expectedCustomMatcher = gstruct.Ignore()
	}
	partialStatusMatchers := []types.GomegaMatcher{expectedCustomMatcher}

	expectedStateMatcher := gstruct.Ignore()
	if expected.State != nil {
		expectedStateMatcher = gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"State": gomega.Equal(*expected.State),
		})
	}
	partialStatusMatchers = append(partialStatusMatchers, expectedStateMatcher)

	expectedReasonMatcher := gstruct.Ignore()
	if expected.Reason != "" {
		expectedReasonMatcher = gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Reason": gomega.Equal(expected.Reason),
		})
	}
	partialStatusMatchers = append(partialStatusMatchers, expectedReasonMatcher)

	expectedReportedByMatcher := gstruct.Ignore()
	if expected.ReportedBy != "" {
		expectedReportedByMatcher = gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"ReportedBy": gomega.Equal(expected.ReportedBy),
		})
	}
	partialStatusMatchers = append(partialStatusMatchers, expectedReportedByMatcher)

	// Matcher for subresource statuses
	expectedSubresourceStatusesMatcher := gstruct.Ignore()
	if expected.SubresourceStatuses != nil {
		subresourceMatchers := make(map[string]types.GomegaMatcher)
		for subresourceKey, subresourceStatus := range expected.SubresourceStatuses {
			subresourceMatchers[subresourceKey] = gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Reason":     gomega.ContainSubstring(subresourceStatus.Reason),
				"ReportedBy": gomega.Equal(subresourceStatus.ReportedBy),
				"State":      gomega.Equal(subresourceStatus.State),
			})
		}
		expectedSubresourceStatusesMatcher = gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"SubresourceStatuses": gstruct.MatchFields(gstruct.IgnoreExtras, subresourceMatchers),
		})
	}
	partialStatusMatchers = append(partialStatusMatchers, expectedSubresourceStatusesMatcher)

	return &HaveStatusMatcher{
		Expected:      expected,
		statusMatcher: gomega.And(partialStatusMatchers...),
	}
}

type HaveStatusMatcher struct {
	Expected      *SoloKitStatus
	statusMatcher types.GomegaMatcher
	// An internal utility for tracking whether we have evaluated this matcher
	// There is a comment within the Match method, outlining why we introduced this
	evaluated bool
}

func (m *HaveStatusMatcher) Match(actual interface{}) (success bool, err error) {
	if m.evaluated {
		// Matchers are intended to be short-lived, and we have seen inconsistent behaviors
		// when evaluating the same matcher multiple times.
		// For example, the underlying http body matcher caches the response body, so if you are wrapping this
		// matcher in an Eventually, you need to create a new matcher each iteration.
		// This error is intended to help prevent developers hitting this edge case
		return false, eris.New("using the same matcher twice can lead to inconsistent behaviors")
	}
	m.evaluated = true

	if ok, err := m.statusMatcher.Match(actual); !ok {
		return false, err
	}

	return true, nil
}

func (m *HaveStatusMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.statusMatcher.FailureMessage(actual),
		informativeComparison(m.Expected, actual))
}

func (m *HaveStatusMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s \n%s",
		m.statusMatcher.NegatedFailureMessage(actual),
		informativeComparison(m.Expected, actual))
}

type HaveNamespacedStatusesMatcher struct {
	Expected                   *SoloKitNamespacedStatuses
	namespacedStatusesMatchers map[string]types.GomegaMatcher
	// An internal utility for tracking whether we have evaluated this matcher
	// There is a comment within the Match method, outlining why we introduced this
	evaluated bool
}

func (m *HaveNamespacedStatusesMatcher) Match(actual interface{}) (success bool, err error) {
	if m.evaluated {
		// Matchers are intended to be short-lived, and we have seen inconsistent behaviors
		// when evaluating the same matcher multiple times.
		// For example, the underlying http body matcher caches the response body, so if you are wrapping this
		// matcher in an Eventually, you need to create a new matcher each iteration.
		// This error is intended to help prevent developers hitting this edge case
		return false, eris.New("using the same matcher twice can lead to inconsistent behaviors")
	}
	m.evaluated = true

	val, ok := actual.(core.NamespacedStatuses)
	if !ok {
		return false, eris.Errorf("matcher expected core.NamespacedStatuses, got %T", actual)
	}

	for ns, matcher := range m.namespacedStatusesMatchers {
		actualStatus, ok := val.GetStatuses()[ns]
		if !ok {
			return false, eris.Errorf("have matcher for namespace %s which is not found", ns)
		}
		if actualStatus == nil {
			return false, eris.New("got nil status")
		}
		if ok, err := matcher.Match(*actualStatus); !ok {
			return false, err
		}
	}

	return true, nil
}

func (m *HaveNamespacedStatusesMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s",
		informativeComparison(m.Expected, actual))
}

func (m *HaveNamespacedStatusesMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("%s",
		informativeComparison(m.Expected, actual))
}
