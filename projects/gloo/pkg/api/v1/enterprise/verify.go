package enterprise

// This is a workaround to verify that all the generated proto files that are not used in this repository are valid
import (
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/jwt"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/rbac"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/waf"
)
