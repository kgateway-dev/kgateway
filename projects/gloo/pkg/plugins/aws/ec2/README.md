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

# TODO
## Port selection
- Currently, the upstream is hard coded to use port 80 on the EC2 instances.
  - This should be configurable.
  - Perhaps by a special tag or by a field on the upstream.
## Private IP Address
- Currently, the upstream routes to the instance's public IP address.
  - Add a config setting to choose the private IP