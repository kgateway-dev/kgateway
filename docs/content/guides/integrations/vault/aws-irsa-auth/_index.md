---
title: Securing secrets in Hashicorp Vault using AWS IAM Roles for Service Accounts (IRSA)
description: Secure your secrets using AWS IAM Roles for Service Accounts (IRSA)
weight: 1
---

Vault supports AWS IAM roles for authentication, offering a choice between hard-coded, long-lived credentials and automated AWS IAM Roles for Service Accounts (IRSA).

AWS IAM Roles for Service Accounts (IRSA) enable you to associate IAM roles with Kubernetes Service Accounts, allowing automatic retrieval and use of temporary AWS credentials.
This integration enhances security and operational efficiency, ensuring Kubernetes applications securely access Vault secrets while following AWS IAM best practices.

## AWS

Start by creating the necessary permissions in AWS.

### Step 1: Create a cluster with OIDC

To configure IRSA, create or use an EKS cluster with an associated OpenID Provider (OIDC). Be sure to use a cluster name that has not previously been used to create the AWS vault permissions in this guide. For more information about creating an EKS cluster in AWS, see [Getting started with Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html).

1. Save the following details in environment variables.
   ```shell
   export NAMESPACE=gloo-system
   export CLUSTER_NAME=<cluster_name>
   export AWS_REGION=<region>
   export ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)
   ```

2. Associate an OIDC provider with your cluster. The output `created IAM Open ID Connect provider for cluster` means that you successfully associated an OIDC provider, and the output `IAM Open ID Connect provider is already associated with cluster` means that an OIDC provider already existed for the cluster. For more information, see [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) in the AWS docs.
   ```shell
   eksctl utils associate-iam-oidc-provider --cluster ${CLUSTER_NAME} --approve
   ```

3. Save the OIDC provider for your cluster in an environment variable.
   ```shell
   export OIDC_PROVIDER=$(aws eks describe-cluster --name ${CLUSTER_NAME} --region ${AWS_REGION} --query "cluster.identity.oidc.issuer" --output text | sed -e "s/^https:\/\///")
   ```

### Step 2: Set up a Role

Create an AWS Role with a trust relationship to your OIDC provider. The provider can assume the role for the `gloo` and `discovery` service accounts.

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
export VAULT_AUTH_ROLE_ARN=$(aws iam create-role \
                --role-name $VAULT_AUTH_ROLE_NAME \
                --assume-role-policy-document file://trust-relationship.json \
                --description "Vault auth role" | jq -r .Role.Arn)

# remove the created file
rm -f trust-relationship.json
```

{{% notice note %}}
If you see an `EntityAlreadyExists` error, then a role with the same name already exists. Use a different `$VAULT_AUTH_ROLE_NAME` than `dev-role-iam-${CLUSTER_NAME}`, and try again. If you want to instead inspect and modify the existing role, first ensure that you understand how it is being used before making any changes, and then run `export VAULT_AUTH_ROLE_ARN=$(aws iam list-roles --query "Roles[?RoleName=='${VAULT_AUTH_ROLE_NAME}'].Arn"--output text)` to set the `$VAULT_AUTH_ROLE_ARN` environment variable.
{{% /notice %}}

### Step 3: Set a Policy

1. Create an AWS Policy to grant the necessary permissions for Vault to perform actions, such as assuming the IAM role and getting instance and user information. This is a lighter version of Vault's [Recommended Vault IAM Policy](https://developer.hashicorp.com/vault/docs/auth/aws#recommended-vault-iam-policy).
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

   export VAULT_AUTH_POLICY_ARN=$(aws iam create-policy \
           --region=${AWS_REGION} \
           --policy-name="${VAULT_AUTH_POLICY_NAME}" \
           --description="Policy used by the Vault user to check instance identity" \
           --policy-document file://gloo-vault-auth-policy.json | jq -r .Policy.Arn)

   rm -f gloo-vault-auth-policy.json
   ```

   {{% notice note %}}
   If you see an `EntityAlreadyExists` error, then a policy with the same name already exists. Use a different `$VAULT_AUTH_POLICY_NAME` than `gloo-vault-auth-policy-${CLUSTER_NAME}`, and try again. If you want to instead inspect and modify the existing policy, first ensure that you understand how it is being used before making any changes, and then run `eexport VAULT_AUTH_POLICY_ARN=$(aws iam list-policies --query "Policies[?PolicyName=='${VAULT_AUTH_POLICY_NAME}'].Arn" --output text)` to set the `$VAULT_AUTH_POLICY_ARN` environment variable.
   {{% /notice %}}

2. Attach the policy to the role that you created earlier.
   ```shell
   aws iam attach-role-policy --role-name=${VAULT_AUTH_ROLE_NAME} --policy-arn=${VAULT_AUTH_POLICY_ARN}
   ```

## Vault

After you set up your AWS resources, you can configure Vault with AWS authentication.

### Step 1: Set up Vault

Deploy an instance of Vault to your cluster. Note that this guide is tested only with Vault installed in the same EKS cluster as in the [AWS section](#aws).

1. Install Vault by choosing one of the installation methods in Vault's [Installing Vault](https://developer.hashicorp.com/vault/docs/install) documentation. The following example uses the basic approach from the Helm installation method. Note that this method uses [dev server mode](https://developer.hashicorp.com/vault/docs/concepts/dev-server), and is intended for testing use with this guide only. 
   ```shell
   helm repo add hashicorp https://helm.releases.hashicorp.com
   helm repo update

   helm install vault hashicorp/vault --set "server.dev.enabled=true" --namespace vault --create-namespace
   ```

2. When the following command returns a table of key-value pairs, Vault is running and ready to use.
   ```shell
   kubectl exec -n vault  vault-0 -- vault status
   ```

### Step 2: Enable AWS authentication and create a Vault policy

Enable AWS authentication and a secrets engine for Vault, and create a Vault policy.

1. Log in to the Vault pod to open a shell.
   ```shell
   kubectl exec -n vault -it vault-0 -- sh
   ```

2. Enable AWS authentication for Vault.
   ```shell
   vault auth enable aws
   ```

3. Enable a secrets engine for Vault.
   ```shell
   vault secrets enable -path="dev" -version=2 kv
   ```

4. Create a Vault policy.
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

5. To log out of the Vault pod, enter `exit`.

### Step 3: Configure the AWS authentication method

Configure Vault's AWS authentication method to point to the Security Token Service (STS) endpoint for your provider. Run these steps outside the Vault pod, because the `kubectl` command uses the environment variables that you set earlier. In later steps, you add an `iam_server_id_header_value` to secure the authN/authZ process and ensure that it matches with your configuration in Gloo. For more information on the IAM Server ID header, see the Vault [API docs](https://developer.hashicorp.com/vault/api-docs/auth/aws#iam_server_id_header_value).

```shell
export IAM_SERVER_ID_HEADER_VALUE=vault.gloo.example.com
kubectl -n vault exec vault-0 -- vault write auth/aws/config/client \
	iam_server_id_header_value=${IAM_SERVER_ID_HEADER_VALUE} \
	sts_endpoint=https://sts.${AWS_REGION}.amazonaws.com \
	sts_region=${AWS_REGION}
```

### Step 4: Associate the Vault Policy with AWS Role

Bind the Vault authentication and policy to your role in AWS. To use IAM roles, the following command sets the `auth_type` to `iam`.

```shell
kubectl -n vault exec vault-0 -- vault write auth/aws/role/${VAULT_AUTH_ROLE_NAME} \
	auth_type=iam \
    bound_iam_principal_arn="${VAULT_AUTH_ROLE_ARN}" \
    policies=dev \
    max_ttl=15m
```

If this command fails, see [Access denied due to identity-based policies – implicit denial](#access-denied-due-to-identity-based-policies--implicit-denial).

## Gloo Edge

Lastly, install Gloo Edge by using a configuration that allows Vault and IRSA credential fetching.

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

This example uses Edge version `v1.15.3`, but you can use any version later than this.

```shell
export EDGE_VERSION=v1.15.3

helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
helm install gloo gloo/gloo --namespace gloo-system --create-namespace --version ${EDGE_VERSION} --values helm-overrides.yaml
```

## Summary

Gloo Edge now securely accesses Vault secrets using temporary credentials obtained through AWS IAM Roles for Service Accounts (IRSA).
This enhances security, streamlines access control, and simplifies authorization within your Kubernetes environment.

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

To create and associate the necessary policy:

1. Set an environment variable with the assumed role. You can find the value in your error message. In the example above, the `<role-name>` would be `foo-role`.
   ```shell
   export VAULT_ASSUMED_ROLE=<role>
   ```

2. Create the policy and associate it with the role.
   ```shell
   export VAULT_AUTH_GET_ROLE_POLICY_NAME=gloo-vault-auth-get-role-policy-${CLUSTER_NAME}
   cat <<EOF > gloo-vault-auth-policy-get-role.json
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
               "Action": [
                   "sts:AssumeRole"
               ],
               "Resource": [
                   "${VAULT_AUTH_ROLE_ARN}"
               ]
           }
       ]
   }
   EOF

   export VAULT_AUTH_POLICY_ASSUME_ROLE_ARN=$(aws iam create-policy \
           --region=${AWS_REGION} \
           --policy-name="${VAULT_AUTH_GET_ROLE_POLICY_NAME}" \
           --description="Policy used by the Vault assumed role to access the ${VAULT_AUTH_ROLE_NAME} role" \
           --policy-document file://gloo-vault-auth-policy-get-role.json | jq -r .Policy.Arn)

   aws iam attach-role-policy --role-name ${VAULT_ASSUMED_ROLE} --policy-arn=${VAULT_AUTH_POLICY_ASSUME_ROLE_ARN}

   rm gloo-vault-auth-policy-get-role.json
   ```

3. Try to associate the Vault policy with the AWS role again. Note that it might take a few moments for the permissions to propagate.
   ```shell
   kubectl -n vault exec vault-0 -- vault write auth/aws/role/${VAULT_AUTH_ROLE_NAME} \
   	auth_type=iam \
       bound_iam_principal_arn="${VAULT_AUTH_ROLE_ARN}" \
       policies=dev \
       max_ttl=15m
   ```