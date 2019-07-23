package create

import "C"
import (
	"fmt"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	//"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	//"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/azure"
	//"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
	//"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	//"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	//"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	//"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	//"strconv"
	//"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"

	"github.com/solo-io/solo-kit/pkg/errors"
)

const EmptyUpstreamGroupCreateError = "please provide weighted destinations for your upstream group, or use -i to create the upstream group interactively"

func UpstreamGroup(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_GROUP_COMMAND.Use,
		Aliases: constants.UPSTREAM_GROUP_COMMAND.Aliases,
		Short:   "Create an Upstream Group",
		Long: "Upstream groups represent groups of upstreams",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.Top.Interactive {
				return errors.Errorf(EmptyUpstreamGroupCreateError)
			}
			if err := surveyutils.AddUpstreamGroupFlagsInteractive(&opts.Create.InputUpstreamGroup); err != nil {
				return err
			}
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			return createUpstreamGroup(opts)
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func createUpstreamGroup(opts *options.Options) error {
	ug, err := upstreamGroupFromOpts(opts)
	if err != nil {
		return err
	}

	if opts.Create.DryRun {
		return common.PrintKubeCrd(ug, v1.UpstreamGroupCrd)
	}

	ug, err = helpers.MustUpstreamGroupClient().Write(ug, clients.WriteOpts{})
	if err != nil {
		return err
	}

	// TODO(kdorosh) implement!
	// helpers.PrintUpstreamGroups(v1.UpstreamGroupList{ug}, opts.Top.Output)

	return nil
}

func upstreamGroupFromOpts(opts *options.Options) (*v1.UpstreamGroup, error) {
	dest, err := upstreamGroupDestinationsFromOpts(opts.Create.InputUpstreamGroup)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid upstream spec")
	}
	return &v1.UpstreamGroup{
		Metadata:     opts.Metadata,
		Destinations: dest,
	}, nil
}

func upstreamGroupDestinationsFromOpts(input options.InputUpstreamGroup) ([]*v1.WeightedDestination, error) {
	fmt.Println(input.WeightedDestinations)
	destinations := make([]*v1.WeightedDestination, len(input.WeightedDestinations))
	for i, v := range input.WeightedDestinations {
		fmt.Println(v)
		tmp := v
		destinations[i] = &tmp
	}
	fmt.Println(destinations)
	return destinations, nil

	//svcSpec, err := serviceSpecFromOpts(input.ServiceSpec)
	//if err != nil {
	//	return nil, err
	//}
	//spec := &v1.UpstreamSpec{}
	//switch input.UpstreamType {
	//case options.UpstreamType_Aws:
	//	if svcSpec != nil {
	//		return nil, errors.Errorf("%v does not support service spec", input.UpstreamType)
	//	}
	//	if input.Aws.Secret.Namespace == "" {
	//		return nil, errors.Errorf("aws secret namespace must not be empty")
	//	}
	//	if input.Aws.Secret.Name == "" {
	//		return nil, errors.Errorf("aws secret name must not be empty")
	//	}
	//	spec.UpstreamType = &v1.UpstreamSpec_Aws{
	//		Aws: &aws.UpstreamSpec{
	//			Region:    input.Aws.Region,
	//			SecretRef: input.Aws.Secret,
	//		},
	//	}
	//case options.UpstreamType_Azure:
	//	if svcSpec != nil {
	//		return nil, errors.Errorf("%v does not support service spec", input.UpstreamType)
	//	}
	//	if input.Azure.Secret.Namespace == "" {
	//		return nil, errors.Errorf("azure secret namespace must not be empty")
	//	}
	//	if input.Azure.Secret.Name == "" {
	//		return nil, errors.Errorf("azure secret name must not be empty")
	//	}
	//	spec.UpstreamType = &v1.UpstreamSpec_Azure{
	//		Azure: &azure.UpstreamSpec{
	//			FunctionAppName: input.Azure.FunctionAppName,
	//			SecretRef:       input.Azure.Secret,
	//		},
	//	}
	//case options.UpstreamType_Consul:
	//	if input.Consul.ServiceName == "" {
	//		return nil, errors.Errorf("must provide consul service name")
	//	}
	//	spec.UpstreamType = &v1.UpstreamSpec_Consul{
	//		Consul: &consul.UpstreamSpec{
	//			ServiceName: input.Consul.ServiceName,
	//			ServiceTags: input.Consul.ServiceTags,
	//			ServiceSpec: svcSpec,
	//		},
	//	}
	//case options.UpstreamType_Kube:
	//	if input.Kube.ServiceName == "" {
	//		return nil, errors.Errorf("Must provide kube service name")
	//	}
	//
	//	spec.UpstreamType = &v1.UpstreamSpec_Kube{
	//		Kube: &kubernetes.UpstreamSpec{
	//			ServiceName:      input.Kube.ServiceName,
	//			ServiceNamespace: input.Kube.ServiceNamespace,
	//			ServicePort:      input.Kube.ServicePort,
	//			Selector:         input.Kube.Selector.MustMap(),
	//			ServiceSpec:      svcSpec,
	//		},
	//	}
	//case options.UpstreamType_Static:
	//	var hosts []*static.Host
	//	if len(input.Static.Hosts) == 0 {
	//		return nil, errors.Errorf("must provide at least 1 host for static upstream")
	//	}
	//	for _, host := range input.Static.Hosts {
	//		var (
	//			addr string
	//			port uint32
	//		)
	//		parts := strings.Split(host, ":")
	//		switch len(parts) {
	//		case 1:
	//			addr = host
	//			port = 80
	//		case 2:
	//			addr = parts[0]
	//			p, err := strconv.Atoi(parts[1])
	//			if err != nil {
	//				return nil, errors.Wrapf(err, "invalid port on host")
	//			}
	//			port = uint32(p)
	//		default:
	//			return nil, errors.Errorf("invalid host format. format must be IP:PORT or HOSTNAME:PORT " +
	//				"eg www.google.com or www.google.com:80")
	//		}
	//		hosts = append(hosts, &static.Host{
	//			Addr: addr,
	//			Port: port,
	//		})
	//	}
	//	spec.UpstreamType = &v1.UpstreamSpec_Static{
	//		Static: &static.UpstreamSpec{
	//			Hosts:       hosts,
	//			UseTls:      input.Static.UseTls,
	//			ServiceSpec: svcSpec,
	//		},
	//	}
	//}
	//return spec, nil
}