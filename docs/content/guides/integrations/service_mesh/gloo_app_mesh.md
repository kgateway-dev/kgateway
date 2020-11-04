---
title: Gloo Edge and AWS App Mesh
weight: 2
description: Using Gloo Edge as an ingress to AWS App Mesh
---

[AWS App Mesh](https://docs.aws.amazon.com/app-mesh/latest/userguide/what-is-app-mesh.html) is an AWS-native service mesh implementation based on [Envoy Proxy](https://www.envoyproxy.io) making it compatible with a wide range of AWS partner and open source tools. 
AWS App Mesh is a managed and highly available service, AWS manages the service-mesh control plane and you connect up the data plane to the control plane by installing and configuring an Envoy Proxy instance next to your workloads. You can use AWS App Mesh with AWS Fargate, Amazon EC2, Amazon ECS, Amazon EKS, and Kubernetes running on AWS, to better run your application at scale.

Gloo Edge complements service-mesh technology by bringing a powerful "API Gateway" to the edge (or even inside) of your mesh to handle things like:

* Oauth Flows
* Request/Response transformation
* API Aggregation with GraphQL
* Function routing
* And more.

Please see our [FAQ]({{% versioned_link_path fromRoot="/introduction/faq#what-s-the-difference-between-gloo-and-istio" %}}) for more on how Gloo Edge can complement a service mesh.

---

## Getting started with AWS App Mesh

For this guide, we'll assume you want to use AWS App Mesh on Kubernetes (AWS EKS in this case, but it can be any Kubernetes on AWS), but AWS App Mesh is not limited to Kubernetes. 

We recommend you follow this link: [Tutorial: Configure App Mesh Integration with Kubernetes] (https://docs.aws.amazon.com/app-mesh/latest/userguide/mesh-k8s-integration.html) for getting started with AWS App Mesh and setting up the examples.

We will assumed that you use `color-mesh` as the name of your mesh and you deployed your EKS cluster in the `us-east-2 (Ohio)` as the AWS region.

Once you have the examples installed, you should have an environment like this:

```noop
kubectl -n appmesh-demo get pods

NAME                                 READY   STATUS    RESTARTS   AGE
colorgateway-69cd4fc669-xv55k        2/2     Running   0          57m
colorteller-845959f54-tdzjq          2/2     Running   0          57m
colorteller-black-6cc98458db-lq257   2/2     Running   0          57m
colorteller-blue-88bcffddb-vt2ls     2/2     Running   0          57m
colorteller-red-6f55b447db-znk8j     2/2     Running   0          57m
```

Notice that we have Envoy Proxy running next to the workloads.  

You should also verify you have all the Virtual Nodes, Virtual Routers, Routes, and Virtual Services:

```noop
kubectl -n appmesh-demo get virtualservice.appmesh.k8s.aws,virtualnode.appmesh.k8s.aws
NAME                                                       AGE
virtualservice.appmesh.k8s.aws/colorgateway.appmesh-demo   1d
virtualservice.appmesh.k8s.aws/colorteller.appmesh-demo    1d

NAME                                            AGE
virtualnode.appmesh.k8s.aws/colorgateway        1d
virtualnode.appmesh.k8s.aws/colorteller         1d
virtualnode.appmesh.k8s.aws/colorteller-black   1d
virtualnode.appmesh.k8s.aws/colorteller-blue    1d
virtualnode.appmesh.k8s.aws/colorteller-red     1d
```

```noop
aws --region us-east-2 appmesh describe-route --route-name color-route-appmesh-demo \
    --virtual-router-name colorgateway-appmesh-demo  --mesh-name color-mesh
```
```json
{
    "route": {
        "meshName": "color-mesh",
        "metadata": {
            "arn": "arn:aws:appmesh:us-east-2:992143172250:mesh/color-mesh/virtualRouter/colorgateway-appmesh-demo/route/color-route-appmesh-demo",
            "createdAt": 1566324369.64,
            "lastUpdatedAt": 1566324369.64,
            "uid": "25d95b1d-ec98-47d4-be46-aaafbdcedfbf",
            "version": 1
        },
        "routeName": "color-route-appmesh-demo",
        "spec": {
            "httpRoute": {
                "action": {
                    "weightedTargets": [
                        {
                            "virtualNode": "colorgateway-appmesh-demo",
                            "weight": 1
                        }
                    ]
                },
                "match": {
                    "prefix": "/color"
                }
            }
        },
        "status": {
            "status": "ACTIVE"
        },
        "virtualRouterName": "colorgateway-appmesh-demo"
    }
}
```

```bash
aws --region us-east-2 appmesh describe-route --route-name color-route-appmesh-demo \
    --virtual-router-name colorteller-appmesh-demo  --mesh-name color-mesh
```
```json
{
    "route": {
        "meshName": "color-mesh",
        "metadata": {
            "arn": "arn:aws:appmesh:us-east-2:992143172250:mesh/color-mesh/virtualRouter/colorteller-appmesh-demo/route/color-route-appmesh-demo",
            "createdAt": 1566324367.037,
            "lastUpdatedAt": 1566328452.747,
            "uid": "4899bec5-4f80-449c-8ddc-e8ec388a1b56",
            "version": 6
        },
        "routeName": "color-route-appmesh-demo",
        "spec": {
            "httpRoute": {
                "action": {
                    "weightedTargets": [
                        {
                            "virtualNode": "colorteller-appmesh-demo",
                            "weight": 1
                        },
                        {
                            "virtualNode": "colorteller-black-appmesh-demo",
                            "weight": 1
                        },
                        {
                            "virtualNode": "colorteller-blue-appmesh-demo",
                            "weight": 1
                        }
                    ]
                },
                "match": {
                    "prefix": "/"
                }
            },
            "priority": 1
        },
        "status": {
            "status": "ACTIVE"
        },
        "virtualRouterName": "colorteller-appmesh-demo"
    }
}
```

---

## Using Gloo Edge as the Ingress for AWS App Mesh

In our above example, the `colorgateway` service calls the `colorteller` service which has a few variants (`colorteller`, `colorteller-black` , `colorteller-blue`). Both of those services are part of the mesh, and we can control the routing between the components with the mesh. To get traffic into the mesh with a powerful API Gateway like Gloo Edge, all we have to do is the following:

1. Install Gloo Edge
2. Create a Gloo Edge VirtualService
3. Create a Route to where we want to bring traffic into the mesh

Installing Gloo Edge is [covered adequately in other sections]({{% versioned_link_path fromRoot="/installation" %}}) of the documentation.

To accomplish steps 2 and 3, run the following command:


```noop
glooctl add route --path-prefix /appmesh/color \
    --prefix-rewrite /color --dest-name appmesh-demo-colorgateway-9080   
```

Now let's figure out what the right URL is to contact Gloo Edge:

```bash
glooctl proxy url

http://a034a61854c2111e992a70a2a7eb7b9a-398563398.us-east-2.elb.amazonaws.com:80
```
And then call our new API:

```bash
curl $(glooctl proxy url)/appmesh/color
```
```json
{"color":"white", "stats": {"black":0.34,"blue":0.35,"white":0.31}}
```

And there you have it! You now have a powerful L7 Ingress and API Gateway for managing traffic coming into your cluster being served with AWS App Mesh. 

### Limitations

Currently, AWS App Mesh is fairly simple in its capabilities. It does very limited routing, cannot do mTLS, etc. As AWS App Mesh adds more capabilities, we'll integrate deeper. The AWS App Mesh roadmap is [publicly available and can be found on GitHub](https://github.com/aws/aws-app-mesh-roadmap/projects/1).

Even with its current limitations, if you would like to connect multiple meshes together (multiple AWS App Mesh or other heterogeneous implementations like Istio), please check out the [SuperGloo](https://supergloo.solo.io) project where we make it easy to stitch together multiple meshes.
