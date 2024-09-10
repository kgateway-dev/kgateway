# AWS Tests

## E2E Tests
See [e2e-tests](./e2e-tests.md) for more details about e2e tests in general

### Lambda Tests
**These steps can only be taken if you are a Gloo Gateway maintainer**

You will need to do the following to run the [AWS Lambda Tests](/test/e2e/aws_test.go) locally:
1. Obtain an AWS IAM User account that is part of the Solo.io organization
2. Create an AWS access key
    - Under IAM > Users in the AWS console, select the User from step 1
    - Under Summary click "Create access key" to create an access key
    - Save the Access key ID and the associated secret key to be used later
3. Install AWS CLI v2
    - You can check whether you have AWS CLI installed by running `aws --version`
    - Installation guides for various operating systems can be found [here](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html)
4. Configure AWS credentials on your machine
    - You can do this by running `aws configure`
    - You will be asked to provide your Access Key ID and Secret Key from step 2, as well as the default region name and default output format
        - It is critical that you set the default region to `us-east-1`
    - This will create a credentials file at `~/.aws/credentials` on Linux or macOS, or at `C:\Users\USERNAME\.aws\credentials` on Windows. 
    - Copy the credentials file to a location in the `gloo` directory, for example at `/test/e2e/aws_credentials/credentials` and set the `AWS_SHARED_CREDENTIALS_FILE` var to that location, relative to `/test/e2e`
      - The tests read this file in order to interact with lambdas that have been created in the Solo.io organization
    - Set the `AWS_PROFILE` env var to the name of the IAM User 
