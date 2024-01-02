---
title: Securing secrets in Hashicorp Vault using AWS IAM Roles for Service Accounts (IRSA)
description: Secure your secrets using AWS IAM Roles for Service Accounts (IRSA)
weight: 1
---

Vault supports AWS IAM roles for authentication, offering a choice between hard-coded long-lived credentials and automated AWS IAM Roles for Service Accounts (IRSA).

AWS IAM Roles for Service Accounts (IRSA) enable you to associate IAM roles with Kubernetes Service Accounts, allowing automatic retrieval and use of temporary AWS credentials.
This integration enhances security and operational efficiency, ensuring Kubernetes applications securely access Vault secrets while following AWS IAM best practices.

## AWS

Start by creating the necessary permissions in AWS.

### Step 1: Create a cluster with OIDC

To configure IRSA, create or use an EKS cluster with an associated OpenID Provider (OIDC). For setup instructions, follow [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) in the AWS docs. Only the first page of the linked guide is neccessary.

These environment variables may be set before you setup your cluster. Use a `CLUSTER_NAME` that is not already in use.
```shell
export NAMESPACE=gloo-system
# use the cluster name created above
export CLUSTER_NAME="gloo-ee-vault-integration"
export AWS_REGION="us-east-1"
export ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
```

This environment variable can only be set after setup
```shell
export OIDC_PROVIDER=$(aws eks describe-cluster --name $CLUSTER_NAME --region $AWS_REGION --query "cluster.identity.oidc.issuer" --output text | sed -e "s/^https:\/\///")
```

At this point double check that you have OIDC provider associated with the cluster. The output should be either match `created IAM Open ID Connect provider for cluster ` or `IAM Open ID Connect provider is already associated with cluster`
```shell
eksctl utils associate-iam-oidc-provider --cluster $CLUSTER_NAME --approve
```

### Step 2: Set up a Role

Create an AWS Role with a trust relationship to your OIDC provider. This allows the provider to assume the AWS IAM role, specifically for the service accounts in `gloo` and `discovery`.

```shell
cat <<EOF > trust-relationship.json
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": {
				"StringEqualsIfExists": {
					"${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
					"${OIDC_PROVIDER}:sub": [
						"system:serviceaccount:${NAMESPACE}:gloo",
						"system:serviceaccount:${NAMESPACE}:discovery"
					]
				}
			}
		}
	]
}
EOF

export VAULT_AUTH_ROLE_NAME="dev-role-iam-${CLUSTER_NAME}"
export VAULT_AUTH_ROLE_ARN=$([[ $(aws iam list-roles --query "Roles[?RoleName=='${VAULT_AUTH_ROLE_NAME}'].Arn" --output text) == "" ]] \
	&& aws iam create-role \
		--role-name $VAULT_AUTH_ROLE_NAME \
		--assume-role-policy-document file://trust-relationship.json \
		--description "Vault auth role" | jq -r .Role.Arn || aws iam list-roles --query "Roles[?RoleName=='${VAULT_AUTH_ROLE_NAME}'].Arn" --output text)

# remove the created file
rm -f trust-relationship.json
```

### Step 3: Set a Policy

Create an AWS Policy to grant the necessary permissions for Vault to perform actions, such as assuming the IAM role and getting instance and user information. This is a lighter version of Vault's [Recommended Vault IAM Policy](https://developer.hashicorp.com/vault/docs/auth/aws#recommended-vault-iam-policy).

```shell
export VAULT_AUTH_POLICY_NAME=gloo-vault-auth-policy-${CLUSTER_NAME}
cat <<EOF > gloo-vault-auth-policy.json
{
	"Version": "2012-10-17",
	"Statement": [
        {
			"Sid": "",
			"Effect": "Allow",
			"Action": [
				"iam:GetInstanceProfile",
				"ec2:DescribeInstances",
				"iam:GetUser",
				"iam:GetRole"
			],
			"Resource": "*"
		},
		{
			"Effect": "Allow",
			"Action": ["sts:AssumeRole"],
			"Resource": ["${VAULT_AUTH_ROLE_ARN}"]
		}
	]
}
EOF

export VAULT_AUTH_POLICY_ARN=$([[ $(aws iam list-policies --query "Policies[?PolicyName=='${VAULT_AUTH_POLICY_NAME}'].Arn" --output text) == "" ]] \
    && aws iam create-policy \
        --region=${AWS_REGION} \
        --policy-name="${VAULT_AUTH_POLICY_NAME}" \
        --description="Policy used by the Vault user to check instance identity" \
        --policy-document file://gloo-vault-auth-policy.json | jq -r .Policy.Arn || aws iam list-policies --query "Policies[?PolicyName=='${VAULT_AUTH_POLICY_NAME}'].Arn" --output text)

rm -f gloo-vault-auth-policy.json
```

Finally, attach the newly-created policy to the role that you created earlier.
```shell
aws iam attach-role-policy --role-name $VAULT_AUTH_ROLE_NAME --policy-arn=${VAULT_AUTH_POLICY_ARN}
```

## Vault

After you set up your AWS resources, you can configure Vault with AWS authentication. This guide has only been verfied with Vault installed in the same EKS cluster we created in the [AWS section](#aws).

### Step 1: Set up Vault

Install Vault by choosing one of the installation methods in Vault's [Installing Vault](https://developer.hashicorp.com/vault/docs/install) documentation.

This is a basic approach that may work for you. Note that is uses dev mode and is intended for use with this guide only. (https://developer.hashicorp.com/vault/docs/concepts/dev-server)

```shell
helm repo add hashicorp https://helm.releases.hashicorp.com
helm repo update

helm install vault hashicorp/vault --set "server.dev.enabled=true" --namespace vault --create-namespace
```

When the following command returns a table of key-value pairs, vault is up and running and ready to use.
```shell
kubectl exec -n vault  vault-0 -- vault status
```

### Step 2: Enable AWS authentication on Vault
Open a shell on the vault pod for steps 2-4:

```shell
kubectl exec -n vault -it vault-0 -- sh
```
Then:

```shell
vault auth enable aws
```

### Step 3: Enable a secrets engine

```shell
vault secrets enable -path="dev" -version=2 kv
```

### Step 4: Create a Vault Policy

```shell
cd
cat <<EOF > policy.hcl
# Access to dev path
path "dev/*" {
	capabilities = ["create", "read", "update", "delete", "list"]
} 

# Additional access for UI
path "dev/" {
	capabilities = ["list"]
}

path "sys/mounts" {
	capabilities = ["read", "list"]
}
EOF

vault policy write dev policy.hcl
rm -f policy.hcl
```

At this point you can log out of the vault pod:
```shell
exit
```

### Step 5: Configure the AWS authentication method

Next, configure Vault's AWS authentication method to point to the Security Token Service (STS) endpoint for your provider. We will run these steps from outside the pod using `kubectl` because we rely on environment variables we have set above.

In later steps, you add an `iam_server_id_header_value` to secure the authN/authZ process and ensure that it matches with your configuration in Gloo For more information on the IAM Server ID header, see the Vault [API docs](https://developer.hashicorp.com/vault/api-docs/auth/aws#iam_server_id_header_value).

```shell
export IAM_SERVER_ID_HEADER_VALUE=vault.gloo.example.com
kubectl -n vault exec vault-0 -- vault write auth/aws/config/client \
	iam_server_id_header_value=${IAM_SERVER_ID_HEADER_VALUE} \
	sts_endpoint=https://sts.${AWS_REGION}.amazonaws.com \
	sts_region=${AWS_REGION}
```

### Step 6: Associate the Vault Policy with AWS Role

Finally, bind the Vault authentication and policy to your role in AWS. To use IAM roles, the following command sets the `auth_type` to `iam`.

```shell
kubectl -n vault exec vault-0 -- vault write auth/aws/role/${VAULT_AUTH_ROLE_NAME} \
	auth_type=iam \
    bound_iam_principal_arn="${VAULT_AUTH_ROLE_ARN}" \
    policies=dev \
    max_ttl=24h
```

If this fails see [Access denied due to identity-based policies – implicit denial](#access-denied-due-to-identity-based-policies--implicit-denial)


## Gloo Edge

Lastly, install Gloo Edge by using a configuration that allows Vault and IRSA credential fetching. This guide has only been verfied with Vault installed in the same EKS cluster we created in the [AWS section](#aws).

### Step 1: Prepare Helm overrides

Override the default settings to use Vault as a source for managing secrets. To allow for IRSA, add the `eks.amazonaws.com/role-arn` annotations, which reference the roles to assume, to the `gloo` and `discovery` service accounts.

Note that you must adjust the `pathPrefix` options when you use a custom `kv` secrets engine. The value of `root_key` is `gloo` by default and is the correct value for this example. Update `VAULT_ADDRESS` if appropriate.

```shell
export VAULT_ADDRESS=http://vault-internal.vault:8200
cat <<EOF > helm-overrides.yaml
settings:
  secretOptions:
    sources:
      - vault:
          # set to address for the Vault instance
          address: ${VAULT_ADDRESS}
          aws:
            iamServerIdHeader: ${IAM_SERVER_ID_HEADER_VALUE}
            mountPath: aws
            region:  ${AWS_REGION}
          # assumes kv store is mounted on 'dev'
          pathPrefix: dev
      - kubernetes: {}
gloo:
  serviceAccount:
    extraAnnotations: 
      eks.amazonaws.com/role-arn: ${VAULT_AUTH_ROLE_ARN}
discovery:
  serviceAccount:
    extraAnnotations:
      eks.amazonaws.com/role-arn: ${VAULT_AUTH_ROLE_ARN}
EOF
```

{{% notice note %}}
If you use Gloo Edge Enterprise, nest these Helm settings within the `gloo` section.
{{% /notice %}}

### Step 2: Install Gloo using Helm

```shell
export EDGE_VERSION=v1.15.3

helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
helm install gloo gloo/gloo --namespace gloo-system --create-namespace --version $EDGE_VERSION --values helm-overrides.yaml
```

## Troubleshooting

### Access denied due to identity-based policies – implicit denial

When you register the role in Vault by running `vault write auth/aws/role/<role name>`, you might encounter the following error due to insufficient action with the identity-based policy.

```
Error writing data to auth/aws/role/dev-role-iam: Error making API request.

URL: PUT http://localhost:8200/v1/auth/aws/role/dev-role-iam
Code: 400. Errors:

* unable to resolve ARN "arn:aws:iam::account-id:role/dev-role-iam" to internal ID: AccessDenied: User: arn:aws:sts::account-id:assumed-role/foo-role/bar is not authorized to perform: iam:GetRole on resource: role dev-role-iam because no identity-based policy allows the iam:GetRole action
	status code: 403, request id: e348ee87-6d44-493b-8763-14fff6aea689
```

To resolve this issue, add the `iam:GetRole` action to a policy attached to the assumed-role identity. In the previous example, you would add the `iam:GetRole` action to the identity `arn:aws:sts::account-id:assumed-role/foo-role/bar`.

One way to do this is via the console:
* Log into the AWS console and go to the IAM dashboard
* Search for and select the role which is "not authorized", in this example `foo-role`
* Go to the `Permissions` tab, which should be the first/default tab
* Find the `Add Permissions` dropdown and select `Create Inline Policy`
* It will ask you to choose a service. Pick `IAM`
* A new `Actions Allowed` menu will appear. Expand the `Read` section and select `GetRole`
* In the `Resources` section below, select `Specific` and follow the `Add ARNs to restrict access` link
* Choose the text option, and add the arn, `arn:aws:iam::account-id:role/dev-role-iam` in this example. Click the `Add ARNS` button
* The overlay window will close, and you can select `Next`
* Choose a policy name and click `Create Policy`
* Done!


## Summary

Now, Gloo Edge securely accesses Vault secrets using temporary credentials obtained through AWS IAM Roles for Service Accounts (IRSA).
This enhances security, streamlines access control, and simplifies authorization within your Kubernetes environment.