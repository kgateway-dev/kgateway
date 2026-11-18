package bootstrappatch

// protojson resolves each `@type` in a bootstrap through Go's global type registry, so
// every type a bootstrap or an overlay uses must be linked into the binary that parses
// it. This package links every go-control-plane type, and the cncf/xds TypedStruct
// wrappers, through pkg/utils/filter_types, so the parse never rejects a type the
// proxy's Envoy supports. A type the proxy's Envoy is not built with, such as a contrib
// extension, parses here and is rejected by Envoy's validation.
// TestGeneratedBootstrapParses in pkg/kgateway/deployer checks that these are linked.
import (
	_ "github.com/kgateway-dev/kgateway/v2/pkg/utils/filter_types"
)
