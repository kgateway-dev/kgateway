package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/spf13/pflag"
)

//set.StringVarP(&upstream.UpstreamType, "type", "t", "",
//	"type of upstream. Available: "+strings.Join(options.UpstreamTypes, " | "))

func AddCreateUpstreamFlags(set *pflag.FlagSet, upstreamType string, upstream *options.InputUpstream) {
	var addServiceSpecFlags bool

	switch upstreamType {
	case options.UpstreamType_Aws:
		set.StringVar(&upstream.Aws.Region, "aws-region", "us-east-1",
			"region for AWS services this upstream utilize")
		set.StringVar(&upstream.Aws.Secret.Name, "aws-secret-name", "",
			"name of a secret containing AWS credentials created with glooctl. See `glooctl create secret aws --help` "+
				"for help creating secrets")
		set.StringVar(&upstream.Aws.Secret.Namespace, "aws-secret-namespace", defaults.GlooSystem,
			"namespace where the AWS secret lives. See `glooctl create secret aws --help` "+
				"for help creating secrets")
	case options.UpstreamType_AwsEc2:
		set.StringVar(&upstream.AwsEc2.Region, "aws-region", "us-east-1",
			"region for AWS services this upstream utilize")
		set.StringVar(&upstream.AwsEc2.Secret.Name, "aws-secret-name", "",
			"name of a secret containing AWS credentials created with glooctl. See `glooctl create secret aws --help` "+
				"for help creating secrets")
		set.StringVar(&upstream.AwsEc2.Secret.Namespace, "aws-secret-namespace", defaults.GlooSystem,
			"namespace where the AWS secret lives. See `glooctl create secret aws --help` "+
				"for help creating secrets")
		set.StringVar(&upstream.AwsEc2.Role, "aws-role-arn", "",
			"Amazon Resource Number (ARN) of role that Gloo should assume on behalf of the upstream")
		set.BoolVar(&upstream.AwsEc2.PublicIp, "public-ip", false,
			"use instance's public IP address")
		set.Uint32Var(&upstream.AwsEc2.Port, "ec2-port", ec2.DefaultPort,
			"port to use to connect to the EC2 instance (default 80)")
		set.StringSliceVar(&upstream.AwsEc2.KeyFilters, "tag-key-filters", nil,
			"list of tag keys that must exist on EC2 instances associated with this upstream")
		set.StringSliceVar(&upstream.AwsEc2.KeyValueFilters.Entries, "tag-key-value-filters", nil,
			"list of tag keys and corresponding values that must exist on EC2 instances associated with this upstream")
	case options.UpstreamType_Azure:
		set.StringVar(&upstream.Azure.FunctionAppName, "azure-app-name", "",
			"name of the Azure Functions app to associate with this upstream")
		set.StringVar(&upstream.Azure.Secret.Name, "azure-secret-name", "",
			"name of a secret containing Azure credentials created with glooctl. See `glooctl create secret azure --help` "+
				"for help creating secrets")
		set.StringVar(&upstream.Azure.Secret.Namespace, "azure-secret-namespace", defaults.GlooSystem,
			"namespace where the Azure secret lives. See `glooctl create secret azure --help` "+
				"for help creating secrets")
	case options.UpstreamType_Consul:
		addServiceSpecFlags = true
		set.StringVar(&upstream.Consul.ServiceName, "consul-service", "",
			"name of the service in the consul registry")
		set.StringSliceVar(&upstream.Consul.ServiceTags, "consul-service-tags", []string{},
			"comma-separated list of tags for choosing a subset of the service in the consul registry")
	case options.UpstreamType_Kube:
		addServiceSpecFlags = true
		set.StringVar(&upstream.Kube.ServiceName, "kube-service", "",
			"name of the kubernetes service")
		set.StringVar(&upstream.Kube.ServiceNamespace, "kube-service-namespace", "default",
			"namespace where the kubernetes service lives")
		set.Uint32Var(&upstream.Kube.ServicePort, "kube-service-port", 80,
			"the port exposed by the kubernetes service. for services with multiple ports, "+
				"create an upstream for each port.")
		set.StringSliceVar(&upstream.Kube.Selector.Entries, "kube-service-labels", []string{},
			"comma-separated list of labels (key=value) to use for customized selection of pods for this upstream. can be used to select subsets of "+
				"pods for a service e.g. for blue-green deployment")
	case options.UpstreamType_Static:
		addServiceSpecFlags = true
		set.StringSliceVar(&upstream.Static.Hosts, "static-hosts", []string{},
			"comma-separated list of hosts for the static upstream. these are hostnames or ips provided in the format "+
				"IP:PORT or HOSTNAME:PORT. if :PORT is missing, it will default to :80")
		set.Var(&upstream.Static.UseTls, "static-outbound-tls",
			"connections Gloo manages to this cluster will attempt to use TLS for outbound connections. "+
				"If not specified, Gloo will automatically set this to true for port 443")
	}

	if addServiceSpecFlags {
		set.StringVar(&upstream.ServiceSpec.ServiceType, "service-spec-type", "",
			"if set, Gloo supports additional routing features to upstreams with a service spec. "+
				"The service spec defines a set of features ")
	}
}
