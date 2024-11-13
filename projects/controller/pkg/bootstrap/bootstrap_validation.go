package bootstrap

import (
	"context"

	"github.com/golang/protobuf/proto"
	envoybootstrap "github.com/solo-io/gloo/pkg/utils/envoyutils/bootstrap"
	envoyvalidation "github.com/solo-io/gloo/pkg/utils/envoyutils/validation"
)

// ValidateBootstrap is deprecated. Please use the functionality exported from the
// packages in pkg/utils/envoyutils
func ValidateBootstrap(
	ctx context.Context,
	filterName string,
	msg proto.Message,
) error {
	bootstrapYaml, err := envoybootstrap.FromFilter(filterName, msg)
	if err != nil {
		return err
	}

	return envoyvalidation.ValidateBootstrap(ctx, bootstrapYaml)
}
