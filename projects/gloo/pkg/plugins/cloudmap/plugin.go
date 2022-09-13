package cloudmap

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
)

const (
	ExtensionName = "cloudmap"
)

type plugin struct {
	settings *v1.Settings
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	if _, ok := in.GetUpstreamType().(*v1.Upstream_Cloudmap); !ok {
		return nil
	}
	xds.SetEdsOnCluster(out, p.settings)
	return nil
}

func (*plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	ctx := opts.Ctx

	results := make(chan v1.EndpointList)

	errorsDuringUpdate := make(chan error)

	go func() {
		// once this goroutine exits, we should close our output channels
		defer close(results)
		defer close(errorsDuringUpdate)

		// poll indefinitely
		for {
			select {
			case <-ctx.Done():
				// context was cancelled, stop polling
				return
			default:
				endpoints, err := getLatestEndpoints(upstreamsToTrack, writeNamespace)
				if err != nil {
					// send the error to Gloo Edge for logging
					errorsDuringUpdate <- err
				} else {
					// send the latest set of endpoints to Gloo Edge
					results <- endpoints
				}

				// sleep 10s between polling
				time.Sleep(time.Second * 10)
			}
		}
	}()

	return results, errorsDuringUpdate, nil
}

// though required by the plugin interface, this function is not necesasary for our plugin
func (*plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, nil
}

// though required by the plugin interface, this function is not necesasary for our plugin
func (*plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	return false, nil
}

func getLatestEndpoints(upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {
	var result v1.EndpointList
	ctx := context.Background()

	for _, us := range upstreams {
		cmSpec := us.GetCloudmap()
		if cmSpec == nil {
			continue
		}
		client, err := getCmClient(ctx, cmSpec.GetAwsAccountId(), cmSpec.GetAssumeRoleName(), "eu-central-1")
		if err != nil {
			return nil, errors.Wrapf(err, "create cloudmap client")
		}

		endpoints, err := getEndpointsForUpstream(ctx, client, us, writeNamespace)
		if err != nil {
			return nil, errors.Wrapf(err, "get endpoints for Upstream, accountid: %s, serviceNAme: %s", cmSpec.GetAwsAccountId(), cmSpec.GetServiceName())
		}
		result = append(result, endpoints...)
	}

	return result, nil
}

func getEndpointsForUpstream(ctx context.Context, cmClient *servicediscovery.Client, us *v1.Upstream, writeNamespace string) ([]*v1.Endpoint, error) {
	var result v1.EndpointList
	cmSpec := us.GetCloudmap()
	nsOutput, err := cmClient.ListNamespaces(ctx, &servicediscovery.ListNamespacesInput{})
	if err != nil {
		return nil, errors.Wrapf(err, "list namespaces")
	}
	var ns *types.NamespaceSummary
	for _, n := range nsOutput.Namespaces {
		if *n.Name == cmSpec.GetCloudmapNamespaceName() {
			ns = &n
			break
		}
	}
	if ns == nil {
		return nil, fmt.Errorf("namespace %s not found", cmSpec.GetCloudmapNamespaceName())
	}

	sOutput, err := cmClient.ListServices(ctx, &servicediscovery.ListServicesInput{
		Filters: []types.ServiceFilter{
			{
				Name:   types.ServiceFilterNameNamespaceId,
				Values: []string{*ns.Id},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "list services")
	}

	var cmService *types.ServiceSummary
	for _, s := range sOutput.Services {
		if *s.Name == cmSpec.GetServiceName() {
			cmService = &s
		}
	}
	if cmService == nil {
		return nil, fmt.Errorf("service %s not found", cmSpec.GetServiceName())
	}

	iOutput, err := cmClient.ListInstances(ctx, &servicediscovery.ListInstancesInput{
		ServiceId: cmService.Id,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "list instances")
	}

	upstreamRef := us.GetMetadata().Ref()

	for _, instance := range iOutput.Instances {
		endpointForInstance := &v1.Endpoint{
			Metadata: &core.Metadata{
				Namespace: writeNamespace,
				Name:      *instance.Id,
				Labels:    instance.Attributes,
			},
			Address:   instance.Attributes["AWS_INSTANCE_IPV4"],
			Port:      cmSpec.GetPort(),
			Upstreams: []*core.ResourceRef{upstreamRef},
		}
		result = append(result, endpointForInstance)
	}
	contextutils.LoggerFrom(ctx).Infof("%+v", result)

	return result, nil
}

func getCmClient(ctx context.Context, accountId, roleName, awsRegion string) (*servicediscovery.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(awsRegion))
	if err != nil {
		return nil, errors.Wrapf(err, "create default config")
	}
	stsclient := sts.NewFromConfig(cfg)

	aro, err := stsclient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, roleName)),
		RoleSessionName: aws.String("gloo"),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "assumeRole")
	}

	cnf, err := awsconfig.LoadDefaultConfig(
		ctx, awsconfig.WithRegion(awsRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(*aro.Credentials.AccessKeyId, *aro.Credentials.SecretAccessKey, *aro.Credentials.SessionToken)),
	)

	if err != nil {
		return nil, errors.Wrapf(err, "create cloudmap config")
	}

	cmClient := servicediscovery.NewFromConfig(cnf)
	return cmClient, nil
}
