package kgateway

// FilterStage represents well-known positions in the HTTP filter chain.
// +kubebuilder:validation:Enum=Fault;AuthN;AuthZ;RateLimit;Route
type FilterStage string

const (
	// FilterStageFault is the earliest stage in the filter chain.
	FilterStageFault FilterStage = "Fault"
	// FilterStageAuthN is the authentication stage.
	FilterStageAuthN FilterStage = "AuthN"
	// FilterStageAuthZ is the authorization stage.
	FilterStageAuthZ FilterStage = "AuthZ"
	// FilterStageRateLimit is the rate limiting stage.
	FilterStageRateLimit FilterStage = "RateLimit"
	// FilterStageRoute is the final processing stage before routing to upstream.
	// The terminal Router filter always runs after this stage.
	FilterStageRoute FilterStage = "Route"
)

// FilterStagePredicate specifies placement relative to a stage.
// +kubebuilder:validation:Enum=Before;During;After
type FilterStagePredicate string

const (
	// FilterStagePredicateBefore places the filter before the specified stage.
	FilterStagePredicateBefore FilterStagePredicate = "Before"
	// FilterStagePredicateDuring places the filter during the specified stage.
	FilterStagePredicateDuring FilterStagePredicate = "During"
	// FilterStagePredicateAfter places the filter after the specified stage.
	FilterStagePredicateAfter FilterStagePredicate = "After"
)

// FilterStageSpec specifies where in the HTTP filter chain a filter should
// be placed.
type FilterStageSpec struct {
	// Stage selects the well-known position in the filter chain.
	// +required
	Stage FilterStage `json:"stage"`

	// Predicate specifies placement relative to the stage: Before, During,
	// or After.
	// +optional
	// +kubebuilder:default=During
	Predicate FilterStagePredicate `json:"predicate,omitempty"`

	// Weight controls ordering among multiple filters at the same
	// stage and predicate. Higher weight places the filter earlier in the
	// chain. Defaults to 0. Filters with the same stage, predicate, and
	// weight are sorted alphabetically by filter name for consistency.
	// +optional
	// +kubebuilder:default=0
	Weight int32 `json:"weight,omitempty"`
}
