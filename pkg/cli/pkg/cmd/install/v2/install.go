package v2

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/v2/crds"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/v2/pkg/deployer"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func install(opts *options.Options, installOpts *Options) error {
	ctx := context.Background()

	valueOpts := &values.Options{
		ValueFiles: installOpts.Values,
		Values:     append(installOpts.Set, "controlPlane.enabled=true", "gateway.enabled=false"),
	}
	helmEnv := cli.New()
	vals, err := valueOpts.MergeValues(getter.All(helmEnv))
	if err != nil {
		return err
	}

	cfg, err := config.GetConfigWithContext(opts.Top.KubeContext)
	if err != nil {
		return err
	}

	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		return err
	}

	if err := gwv1.Install(cli.Scheme()); err != nil {
		return err
	}

	dep, err := deployer.NewDeployer(cli.Scheme(), false, "glooctl", "", 0)
	if err != nil {
		return err
	}

	createNamespace(ctx, cli, installOpts.Namespace)

	// Check if CRDs already exist
	crdsExist, err := detectCrds(cfg)
	if err != nil {
		return err
	}

	if !crdsExist {
		fmt.Printf("Applying Gateway CRDs... ")
		crds, err := deployer.ConvertYAMLToObjects(cli.Scheme(), crds.GatewayCrds)
		if err != nil {
			fmt.Printf("Failed\n")
			return err
		}
		if err := dep.DeployObjs(ctx, crds, cli); err != nil {
			fmt.Printf("Failed\n")
			return err
		}
		fmt.Printf("Done\n")
	} else {
		fmt.Printf("Skipping Gateway CRDs as they exist...\n")
	}

	objs, err := dep.Render(ctx, "default", installOpts.Namespace, vals)
	if err != nil {
		return err
	}

	for _, obj := range objs {
		obj.SetNamespace(installOpts.Namespace)
	}

	fmt.Printf("Applying Manifest... ")
	if err := dep.DeployObjs(ctx, objs, cli); err != nil {
		fmt.Printf("Failed\n")
		return err
	}
	fmt.Printf("Done\n")

	all := gwv1.NamespacesFromAll
	if installOpts.Gateway {
		fmt.Printf("Creating Gateway Object... ")
		if err := cli.Patch(ctx, &gwv1.Gateway{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "gateway.networking.k8s.io/v1",
				Kind:       "Gateway",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "http",
				Namespace: installOpts.Namespace,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "gloo-gateway",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Port:     8080,
						Protocol: "HTTP",
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &all,
							},
						},
					},
				},
			},
		},
			client.Apply,
			client.ForceOwnership,
			client.FieldOwner("glooctl"),
		); err != nil {
			fmt.Printf("Failed\n")
			return err
		}
		fmt.Printf("Done\n")
	}

	fmt.Printf("All resources have been successfully initialized!\n")
	fmt.Printf("Please run glooctl check to make sure everything is up and running :)\n")
	return nil
}

func detectCrds(cfg *rest.Config) (bool, error) {
	discClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, err
	}

	groups, err := discClient.ServerGroups()
	if err != nil {
		return false, err
	}

	// Check if gateway group exists
	for _, group := range groups.Groups {
		if group.Name == "gateway.networking.k8s.io" {
			return true, nil
		}
	}

	return false, nil
}

func createNamespace(ctx context.Context, cli client.Client, namespace string) {
	ns := corev1.Namespace{}
	err := cli.Get(ctx, client.ObjectKey{Name: namespace}, &ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating namespace %s... ", namespace)
			if err := cli.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}); err != nil {
				fmt.Printf("\nUnable to create namespace %s. Continuing...\n", namespace)
			} else {
				fmt.Printf("Done.\n")
			}
		} else {
			fmt.Printf("\nUnable to check if namespace %s exists. Continuing...\n", namespace)
		}
	} else {
		fmt.Printf("Namespace %s already exists. Continuing...\n", namespace)
	}
}
