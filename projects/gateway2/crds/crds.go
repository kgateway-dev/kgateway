package crds

import (
	_ "embed"
)

//go:embed gateway-crds.yaml
var GatewayCrds []byte
