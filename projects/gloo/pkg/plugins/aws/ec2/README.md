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

- Gloo Edge needs AWS credentials to be able to find EC2 resources
- Recommendation: create a set of credentials that only have access to the relevant resources.
  - In this example, pretend that the secret we create only has access to resources with the `gloo-tag:group1` tag.
```bash
glooctl create secret aws \
  --name gloo-tag-group1 \
  --namespace default \
  --access-key [aws_secret_key_id] \
  --secret-key [aws_secret_access_key]
```


## Create roles for Gloo Edge to assume on behalf of your upstreams
- For additional control over Gloo Edge's access to your resources and as an additional filter on your EC2 Upstream's list of
available instances it is recommended that you credential your upstreams with a low-access user account that has the
ability to assume the specific roles it requires.
- When you provide both a secret ref and a list of Role ARNs to your upstream, Gloo Edge will call the AWS API with credentials
composed from that user account and those roles (via the AssumeRole feature).
- To configure you AWS account for this use case, there are two steps to take (if you have not already done so):
  - Create a policy that allows the policy holder to describe EC2 instances
  - Create a role that contains that policy and trusts (ie: grants roles assumption to) the upstream's account 

### Create a role
- In the AWS console:
  - Navigate to IAM > Roles, choose "Create Role"
  - Follow the interactive guide to create a role
    - Choose "AWS account" as the type of trusted entity and provide the 12 digit account id of the account which holds
    the EC2 instances you want to route to.
    
- Choose or create a policy for the role
    - An example of a **Policy** that allows the role to describe EC2 instances is shown below.
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": "ec2:DescribeInstances",
            "Resource": "*"
        }
    ]
}
```

### Allow your upstream's user account to list EC2 instances
- In the AWS console:
  - Navigate to IAM > Roles, Select your role
  - Select the "Trust relationships" tab
      - Note the entries under the "Trusted entities" table
  - Click "Edit trust relationship"
  - Add your user/service account's ARN to the Principal.AWS list, as shown below
- An example of **Trust Relationship** is shown below (many other variants are possible)
  - Add the ARNs of each of the user accounts that you want to allow to assume this role.
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::[account_id]:user/[user_id]"
        ]
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
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
- Alternatively, the user could just provide credentials, and allow Gloo Edge to discover the specs by inspection of the tags.
## Port selection from tag
- Currently, the port is specified on the upstream spec.
- It might be useful to allow the user to define the port through a resource tag
- This would support EC2 upstream discovery
- What tag to use? Would this be defined on the upstream, a setting, or by a constant?


# Notes on configuring user accounts for access to specific instances

- To restrict your upstream to selecting among specific EC2 instances, you need to give it an AWS secret that has a custom policy which limits its access to specific resources.
- AWS provides extensive documentation ([policy docs](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies.html), [EC2 example](https://aws.amazon.com/premiumsupport/knowledge-center/restrict-ec2-iam/)), but we will capture the gist here.

## Sample custom policy

```json
{
   "Version":"2012-10-17",
   "Statement":[
      {
         "Effect":"Allow",
         "Action":"ec2:DescribeInstances",
         "Resource":[
            "arn:aws:ec2:us-east-1:111122223333:instance/*"
         ],
         "Condition":{
            "StringEquals":{
               "ec2:ResourceTag/Owner":"Gloo Edge"
            }
         }
      }
   ]
}
```

### Action
- The action that the EC2 upstream credentials must have is `ec2:DescribeInstances`.
  - [DescribeInstances](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html) is the only AWS API that Gloo Edge needs.
  
### Resource list
- To restrict an upstream's access to a specific set of instances, list them (wildcards supported) by their Amazon Resource Name (ARN).
- For EC2, your resource ARN will have this format:
  - `arn:aws:ec2:[region]:[account-id]:instance:[resource]:[qualifier]`
  - Other variants are possible, refer to the [ARN docs](https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html) for details.

### Conditions
- It is also possible to identify resources by various conditions.
- The `ResourceTags`, in particular, are how Gloo Edge chooses which EC2 instances to associate with a given upstream.
  - Refer to the [policy condition docs](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_condition.html) for details

## Considerations
- AWS has a highly expressive policy definition protocol for restricting an account's access to resources.
- Gloo Edge uses the intersection of an upstream's credentials and its filter spec to determine which EC2 instances should be associated with an upstream.
- You have a few options where to store your config:
  - Permissive upstream credentials (an upstream may be able to list EC2 instances that it should not route to), discerning upstream filters (upstream filters refine the set of target instances)
  - Restrictive upstream credentials (only allow upstream to the credentials that it should route to), no upstream filters
  - Both restrictive upstream credentials and discerning upstream filters (this may serve as a form of documentation or consistency check)

