package create

import (
	"strconv"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func Upstream(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_COMMAND.Use,
		Aliases: constants.UPSTREAM_COMMAND.Aliases,
		Short:   "Create an Upstream Interactively",
		Long: "Upstreams represent destination for routing HTTP requests. Upstreams can be compared to \n" +
			"[clusters](https://www.envoyproxy.io/docs/envoy/latest/api-v1/cluster_manager/cluster.html?highlight=cluster) in Envoy terminology. \n" +
			"Each upstream in Gloo has a type. Supported types include `static`, `kubernetes`, `aws`, `consul`, and more. \n" +
			"Each upstream type is handled by a corresponding Gloo plugin. \n",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := surveyutils.AddUpstreamFlagsInteractive(&opts.Create.InputUpstream); err != nil {
				return err
			}
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			return createUpstream(opts)
		},
	}
	cmd.AddCommand(
		createUpstreamSubcommand(opts,
			options.UpstreamType_Aws,
			"Create an Aws Upstream",
			"AWS Upstreams represent a set of AWS Lambda Functions for a Region that can be routed to with Gloo. "+
				"AWS Upstreams require a valid set of AWS Credentials to be provided. "+
				"These should be uploaded to Gloo using `glooctl create secret aws`",
		),
		createUpstreamSubcommand(opts,
			options.UpstreamType_Azure,
			"Create an Azure Upstream",
			"Azure Upstreams represent a set of Azure Functions for a Function App that can be routed to with Gloo. "+
				"Azure Upstreams require a valid set of Azure Credentials to be provided. "+
				"These should be uploaded to Gloo using `glooctl create secret azure`",
		),
		createUpstreamSubcommand(opts,
			options.UpstreamType_Consul,
			"Create a Consul Upstream",
			"Consul Upstreams represent a collection of endpoints for Services registered with Consul. "+
				"Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, "+
				"if upstream discovery in Gloo is disabled, or ACL permissions have not been granted to Gloo to read from the registry, "+
				"Consul services can be added to Gloo manually via the CLI.",
		),
		createUpstreamSubcommand(opts,
			options.UpstreamType_Kube,
			"Create a Kubernetes Upstream",
			"Kubernetes Upstreams represent a collection of endpoints for Services registered with Kubernetes. "+
				"Typically, Gloo will automatically discover these upstreams, meaning you don't have to create them. However, "+
				"if upstream discovery in Gloo is disabled, or RBAC permissions have not been granted to Gloo to read from the registry, "+
				"Kubernetes services can be added to Gloo manually via the CLI.",
		),
		createUpstreamSubcommand(opts,
			options.UpstreamType_Static,
			"Create a Static Upstream",
			"Static upstreams are intended to connect Gloo to upstreams to services (often external or 3rd-party) "+
				"running at a fixed IP address or hostname. Static upstreams require you to manually specify the hosts associated "+
				"with a static upstream. Requests routed to a static upstream will be round-robin load balanced across each host.",
		),
	)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func createUpstreamSubcommand(opts *options.Options, upstreamType, short, long string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   upstreamType,
		Short: short,
		Long:  long,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if upstreamType == options.UpstreamType_Consul || upstreamType == options.UpstreamType_Kube {
					// Short circuit this error propagation. Before this was getting bubbled up after asking
					// the user to provide metadata, which made for a bad experience. We can remove these checks
					// when we implement interactive mode for these types.
					return errors.Errorf("interactive mode not currently available for type %v", upstreamType)
				}
			}
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			opts.Create.InputUpstream.UpstreamType = upstreamType
			if opts.Top.Interactive {
				if err := surveyutils.AddUpstreamFlagsInteractive(&opts.Create.InputUpstream); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return createUpstream(opts)
		},
	}
	flagutils.AddUpstreamFlags(cmd.PersistentFlags(), upstreamType, &opts.Create.InputUpstream)
	return cmd
}

func createUpstream(opts *options.Options) error {
	us, err := upstreamFromOpts(opts)
	if err != nil {
		return err
	}
	us, err = helpers.MustUpstreamClient().Write(us, clients.WriteOpts{})
	if err != nil {
		return err
	}

	helpers.PrintUpstreams(v1.UpstreamList{us}, opts.Top.Output)

	return nil
}

func upstreamFromOpts(opts *options.Options) (*v1.Upstream, error) {
	spec, err := upstreamSpecFromOpts(opts.Create.InputUpstream)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid upstream spec")
	}
	return &v1.Upstream{
		Metadata:     opts.Metadata,
		UpstreamSpec: spec,
	}, nil
}
func upstreamSpecFromOpts(input options.InputUpstream) (*v1.UpstreamSpec, error) {
	svcSpec, err := serviceSpecFromOpts(input.ServiceSpec)
	if err != nil {
		return nil, err
	}
	spec := &v1.UpstreamSpec{}
	switch input.UpstreamType {
	case options.UpstreamType_Aws:
		if svcSpec != nil {
			return nil, errors.Errorf("%v does not support service spec", input.UpstreamType)
		}
		if input.Aws.Secret.Namespace == "" {
			return nil, errors.Errorf("aws secret namespace must not be empty")
		}
		if input.Aws.Secret.Name == "" {
			return nil, errors.Errorf("aws secret name must not be empty")
		}
		spec.UpstreamType = &v1.UpstreamSpec_Aws{
			Aws: &aws.UpstreamSpec{
				Region:    input.Aws.Region,
				SecretRef: input.Aws.Secret,
			},
		}
	case options.UpstreamType_Azure:
		if svcSpec != nil {
			return nil, errors.Errorf("%v does not support service spec", input.UpstreamType)
		}
		if input.Azure.Secret.Namespace == "" {
			return nil, errors.Errorf("azure secret namespace must not be empty")
		}
		if input.Azure.Secret.Name == "" {
			return nil, errors.Errorf("azure secret name must not be empty")
		}
		spec.UpstreamType = &v1.UpstreamSpec_Azure{
			Azure: &azure.UpstreamSpec{
				FunctionAppName: input.Azure.FunctionAppName,
				SecretRef:       input.Azure.Secret,
			},
		}
	case options.UpstreamType_Consul:
		if input.Consul.ServiceName == "" {
			return nil, errors.Errorf("must provide consul service name")
		}
		spec.UpstreamType = &v1.UpstreamSpec_Consul{
			Consul: &consul.UpstreamSpec{
				ServiceName: input.Consul.ServiceName,
				ServiceTags: input.Consul.ServiceTags,
				ServiceSpec: svcSpec,
			},
		}
	case options.UpstreamType_Kube:
		if input.Kube.ServiceName == "" {
			return nil, errors.Errorf("Must provide kube service name")
		}

		spec.UpstreamType = &v1.UpstreamSpec_Kube{
			Kube: &kubernetes.UpstreamSpec{
				ServiceName:      input.Kube.ServiceName,
				ServiceNamespace: input.Kube.ServiceNamespace,
				ServicePort:      input.Kube.ServicePort,
				Selector:         input.Kube.Selector.MustMap(),
				ServiceSpec:      svcSpec,
			},
		}
	case options.UpstreamType_Static:
		var hosts []*static.Host
		if len(input.Static.Hosts) == 0 {
			return nil, errors.Errorf("must provide at least 1 host for static upstream")
		}
		for _, host := range input.Static.Hosts {
			var (
				addr string
				port uint32
			)
			parts := strings.Split(host, ":")
			switch len(parts) {
			case 1:
				addr = host
				port = 80
			case 2:
				addr = parts[0]
				p, err := strconv.Atoi(parts[1])
				if err != nil {
					return nil, errors.Wrapf(err, "invalid port on host")
				}
				port = uint32(p)
			default:
				return nil, errors.Errorf("invalid host format. format must be IP:PORT or HOSTNAME:PORT " +
					"eg www.google.com or www.google.com:80")
			}
			hosts = append(hosts, &static.Host{
				Addr: addr,
				Port: port,
			})
		}
		spec.UpstreamType = &v1.UpstreamSpec_Static{
			Static: &static.UpstreamSpec{
				Hosts:       hosts,
				UseTls:      input.Static.UseTls,
				ServiceSpec: svcSpec,
			},
		}
	}
	return spec, nil
}

func serviceSpecFromOpts(input options.InputServiceSpec) (*plugins.ServiceSpec, error) {
	var spec *plugins.ServiceSpec
	switch input.ServiceType {
	case options.ServiceType_Rest:
		swaggerInfo := &rest.ServiceSpec_SwaggerInfo{}
		switch {
		case input.InputRestServiceSpec.SwaggerDocInline != "":
			swaggerInfo.SwaggerSpec = &rest.ServiceSpec_SwaggerInfo_Inline{
				Inline: input.InputRestServiceSpec.SwaggerDocInline,
			}
		case input.InputRestServiceSpec.SwaggerUrl != "":
			swaggerInfo.SwaggerSpec = &rest.ServiceSpec_SwaggerInfo_Url{
				Url: input.InputRestServiceSpec.SwaggerUrl,
			}
		default:
			return nil, errors.Errorf("must provide either Swagger URL or Inline Swagger doc")
		}
		spec = &plugins.ServiceSpec{
			PluginType: &plugins.ServiceSpec_Rest{
				Rest: &rest.ServiceSpec{
					SwaggerInfo: swaggerInfo,
				},
			},
		}
	case options.ServiceType_Grpc:
		spec = &plugins.ServiceSpec{
			PluginType: &plugins.ServiceSpec_Grpc{
				Grpc: &grpc.ServiceSpec{
					Descriptors: input.InputGrpcServiceSpec.Descriptors,
				},
			},
		}
	}
	return spec, nil
}
