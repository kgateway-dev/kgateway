package install

import (
	_ "github.com/solo-io/gloo/pkg/plugins/aws"
	_ "github.com/solo-io/gloo/pkg/plugins/azure"
	_ "github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
	_ "github.com/solo-io/gloo/pkg/plugins/consul"
	_ "github.com/solo-io/gloo/pkg/plugins/fake"
	_ "github.com/solo-io/gloo/pkg/plugins/google"
	_ "github.com/solo-io/gloo/pkg/plugins/grpc"
	_ "github.com/solo-io/gloo/pkg/plugins/kubernetes"
	_ "github.com/solo-io/gloo/pkg/plugins/nats-streaming"
	_ "github.com/solo-io/gloo/pkg/plugins/rest"

	_ "github.com/solo-io/gloo/pkg/plugins/connect"
)
