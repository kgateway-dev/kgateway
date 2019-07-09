# EC2 Plugin

This plugin allows you to create upsreams from groups of EC2 instances.

## Sample upstream config

The upstream config below creates an upstream that load balances to all EC2 instances that match the filter spec and are visible to a user with the credentials provided by the secret.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
  name: my-ec2-upstream
  namespace: gloo-system
spec:
  upstreamSpec:
    awsEc2:
      filters:
      - key: some-key
      - kvPair:
          key: some-other-key
          value: some-value
      region: us-east-1
      secretRef:
        name: my-aws-secret
        namespace: default
```
  


# Tutorial: basic use case

- Below is an outline of how to use the EC2 plugin to create routes to EC2 instances.
- Assumption: you have gloo installed as a gateway with the EC2 plugin active.

## Configure an EC2 instance

- Provision an EC2 instance
  - Use an "amazon linux" image
  - Configure the security group to allow http traffic on port 80

- Tag your instance with the following tags
  - gloo-id: abcde123
  - gloo-tag: group1
  - version: v1.2.3

- Set up your EC2 instance
  - download a demo app: an http response code echo app
    - this app responds to requests with the corresponding response code
      - ex: http://<my-instance-ip>/?code=404 produces a `404` response
  - make the app executable
  - run it in the background

```bash
wget https://mitch-solo-public.s3.amazonaws.com/echoapp2
chmod +x echoapp2
sudo ./echoapp2 --port 80 &
```

- Verify that you can reach the app
  - `curl` the app, you should see a help menu for the app
```bash
curl http://<instance-public-ip>/
```

## Create a secret with aws credentials

- Gloo needs AWS credentials to be able to find EC2 resources
- Recommendation: create a set of credentials that only have access to the relevant resources.
  - In this example, pretend that the secret we create only has access to resources with the `gloo-tag:group1` tag.
```bash
glooctl create secret aws \
  --name gloo-tag-group1 \
  --namespace default \
  --access-key <aws_secret_key_id> \
  --secret-key <aws_secret_access_key>
```

## Create an EC2 Upstream

- Make an upstream that points to the resources that you want to route to.
- For this example, we will demonstrate the two ways to build AWS resource filters: by key and by key-value pair.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
  name: ec2-demo-upstream
  namespace: gloo-system
spec:
  upstreamSpec:
    awsEc2:
      filters:
      - key: gloo-id
      - kvPair:
          key: gloo-tag
          value: group1
      - kvPair:
          key: version
          value: v1.2.3
      region: us-east-1
      secretRef:
        name: gloo-tag-group1
        namespace: default
```

## Create a route to your upstream

- Now that you have created an upstream, you can route to it as you would with any other upstream.

```bash
glooctl add route  \
  --path-exact /echoapp  \
  --dest-name ec2-demo-upstream \
  --prefix-rewrite /
```

- Verify that the route works
  - You should see the same output as when you queried the EC2 instance directly.
```bash
export URL=`glooctl proxy url`
curl $URL/echoapp
```


# Potential features, as needed
## Discover upstreams
- The user currently specifies the upstream.
- Alternatively, the user could just provide credentials, and allow Gloo to discover the specs by inspection of the tags.
## Port selection from tag
- Currently, the port is specified on the upstream spec.
- It might be useful to allow the user to define the port through a resource tag
- This would support EC2 upstream discovery
- What tag to use? Would this be defined on the upstream, a setting, or by a constant?
