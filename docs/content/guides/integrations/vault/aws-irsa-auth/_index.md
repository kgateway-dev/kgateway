---
title: Securing Vault secrets with AWS IAM Roles for Service Accounts (IRSA)
description: Secure your secrets using AWS IAM Roles for Service Accounts (IRSA)
weight: 1
---

Vault includes the ability to handle authentication using AWS IAM roles. We can either hard-code long-lived credentials, or infer them using AWS IRSA.

AWS allows the ability to manage credentials through **IAM Roles for Service Accounts** (IRSA). With this feature, we can associate IAM roles to Kubernetes Service Accounts, and gain the ability to fetch and use temporary credentials for said role. More information in AWS' [IAM roles for service account](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) docs.

# AWS

## Step 1: Creating a cluster with OIDC

To configure IRSA, we'll need to have an EKS cluster with an associated OpenID Provider (OIDC). For setup instructions, follow AWS' [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) docs.

Afterward, we'll want to export some environment variables to use throughout our configuration.

```shell
export NAMESPACE=gloo-system
# use the cluster name created above
export CLUSTER_NAME="gloo-ee-vault-integration"
export AWS_REGION="us-east-1"
export ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
export OIDC_PROVIDER=$(aws eks describe-cluster --name $CLUSTER_NAME --region $AWS_REGION --query "cluster.identity.oidc.issuer" --output text | sed -e "s/^https:\/\///")
```

## Step 2: Setting up a Role

We'll want to create an AWS Role with a trust relationship to our OIDC provider. This would allow the AWS IAM role to be assumed by the provider, specifically for the service accounts in `gloo` and `discovery`.

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

export VAULT_AUTH_ROLE_NAME="dev-role-iam"
export VAULT_AUTH_ROLE_ARN=$([[ $(aws iam list-roles --query "Roles[?RoleName=='${VAULT_AUTH_ROLE_NAME}'].Arn" --output text) == "" ]] \
	&& aws iam create-role \
		--role-name $VAULT_AUTH_ROLE_NAME \
		--assume-role-policy-document file://trust-relationship.json \
		--description "Vault auth role" | jq -r .Role.Arn || aws iam list-roles --query "Roles[?RoleName=='${VAULT_AUTH_ROLE_NAME}'].Arn" --output text)

# remove the created file
rm -f trust-relationship.json
```

## Step 3: Setting a Policy

Next we will create an AWS Policy to grant the necessary permissions for Vault, allowing it to perform actions, such as assuming the IAM role and getting instance and user information. This is a lighter version of Vault's [Recommended Vault IAM Policy](https://developer.hashicorp.com/vault/docs/auth/aws#recommended-vault-iam-policy).

```shell
export VAULT_AUTH_POLICY_NAME=gloo-vault-auth-policy
cat <<EOF > gloo-vault-auth-policy.json
{
	"Version": "2012-10-17"
	"Statement": [
        {
			"Sid": "",
			"Effect": "Allow",
			"Action": [
				"iam:GetInstanceProfile",
				"ec2:DescribeInstances"
				"iam:GetUser",
				"iam:GetRole",
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
        --region=ap-southeast-2 \
        --policy-name="${VAULT_AUTH_POLICY_NAME}" \
        --description="Policy used by the Vault user to check instance identity" \
        --policy-document file://gloo-vault-auth-policy.json | jq -r .Policy.Arn || aws iam list-policies --query "Policies[?PolicyName=='${VAULT_AUTH_POLICY_NAME}'].Arn" --output text)

rm -f gloo-vault-auth-policy.json
```

Then finally, attach the newly-created policy to the role created earlier with the following code:
```shell
aws iam attach-role-policy --role-name $VAULT_AUTH_ROLE_NAME --policy-arn=${VAULT_AUTH_POLICY_ARN}
```
# Vault

After setting up our AWS resources, we can start setting up Vault with AWS authentication. To install Vault on your system, choose one of the installation methods in Vault's [Installing Vault](https://developer.hashicorp.com/vault/docs/install) documentation.

## 1.  Enable AWS authentication on Vault

```shell
vault auth enable aws
```

## 2. Enable a secrets engine

```shell
vault secrets enable -path="dev" -version=2 kv
```

## 3. Create a Vault Policy

```shell
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

# 4. Configure the AWS authentication method

We'll then want to configure Vault's AWS authentication method to point to the Security Token Service (STS) endpoint for our provider.

We will be adding an `iam_server_id_header_value` to secure the authN/authZ process and ensuring it matches with our configuration in Gloo later on. For more information regarding the IAM Server ID header, it is explained in Vault's [API Docs](https://developer.hashicorp.com/vault/api-docs/auth/aws#iam_server_id_header_value).

```shell
export IAM_SERVER_ID_HEADER_VALUE=vault.gloo.example.com
vault write auth/aws/config/client \
	iam_server_id_header_value=${IAM_SERVER_ID_HEADER_VALUE} \
	sts_endpoint=https://sts.${AWS_REGION}.amazonaws.com \
	sts_region=${AWS_REGION}
```

## 5. Associating the Vault Policy with AWS Role

Finally, we'll want to bind the Vault authentication and policy to our role created in AWS. Since we are doing IAM roles, we'll set the `auth_type` to `iam`.

```shell
vault write auth/aws/role/dev-role-iam \
	auth_type=iam \
    bound_iam_principal_arn="${VAULT_AUTH_ROLE_ARN}" \
    policies=dev \
    max_ttl=24h
```

# Gloo Edge

Lastly, we'll need to install Gloo Edge configured to allow for Vault and IRSA credential fetching.
## 1. Prepare Helm overrides

We override our default settings to use Vault as a source for managing secrets. To allows for IRSA, we'll need to add the `eks.amazonaws.com/role-arn` annotations to our `gloo` and `discovery` service account referencing the roles we'll assume.

```shell
cat <<EOF > helm-overrides.yaml
settings:
	secretOptions:
		sources:
		- vault:
			# set to address for the Vault instance
			address: http://vault-internal.vault:8200
			aws:
				iamServerIdHeader: ${IAM_SERVER_ID_HEADER_VALUE}
				mountPath: aws
				region: ${AWS_REGION}
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
If using Gloo Edge Enterprise, remember to nest the above helm settings inside `gloo`.
{{% /notice %}}


## 2. Install Gloo (using Helm)

```shell
export EDGE_VERSION=v1.15.2

helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
helm install gloo gloo/gloo --namespace gloo-system --create-namespace --version $EDGE_VERSION --values helm-overrides.yaml
```
