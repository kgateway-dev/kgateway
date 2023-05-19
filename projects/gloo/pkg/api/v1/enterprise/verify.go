package enterprise

// This is a workaround to verify that all the generated proto files that are not used in this repository are valid
import (
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/caching"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/dlp"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/jwt"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/rbac"
	_ "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/waf"
)
