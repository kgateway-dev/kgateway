package v1alpha1

// PolicyConditionType is a type of condition for a policy. This type should be
// used with a Policy resource Status.Conditions field.
type PolicyConditionType string

// PolicyConditionReason is a reason for a policy condition.
type PolicyConditionReason string

const (
	// PolicyConditionAccepted indicates whether the policy has been accepted or rejected, and why.
	//
	// Possible reasons for this condition to be True are:
	// * Valid
	//
	// Possible reasons for this condition to be False are:
	// * Pending
	// * Invalid
	//
	PolicyConditionAccepted PolicyConditionType = "Accepted"

	// PolicyConditionAttached indicates whether the policy has attached to the targeted resources.
	//
	// Possible reasons for this condition to be True are:
	// * Attached
	// * Merged
	//
	// Possible reasons for this condition to be False are:
	// * Pending
	// * Overridden
	//
	PolicyConditionAttached PolicyConditionType = "Attached"

	// PolicyReasonValid is used with the "Accepted" condition when the policy
	// has been accepted by the system.
	PolicyReasonValid PolicyConditionReason = "Valid"

	// PolicyReasonInvalid is used with the "Accepted" or "Attached" condition when the policy
	// is syntactically or semantically invalid.
	PolicyReasonInvalid PolicyConditionReason = "Invalid"

	// PolicyReasonAttached is used with the "Attached" condition when the
	// policy has been successfully attached to all the targeted resources.
	PolicyReasonAttached PolicyConditionReason = "Attached"

	// PolicyReasonMerged is used with the "Attached" condition when the
	// policy has been merged with other policies and attached to the targeted resources.
	PolicyReasonMerged PolicyConditionReason = "Merged"

	// PolicyReasonOverridden is used with the "Attached" condition when the
	// policy is fully overridden on any targeted resource due to a conflict
	// with another policy of higher priority.
	PolicyReasonOverridden PolicyConditionReason = "Overridden"

	// PolicyReasonPending is used with the "Accepted" or "Attached" condition when the policy has been referenced but not yet fully processed by the controller.
	PolicyReasonPending PolicyConditionReason = "Pending"
)

// PolicyDisable is used to disable a policy.
type PolicyDisable struct{}
