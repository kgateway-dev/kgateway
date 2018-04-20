### What you'll need
- Kubernetes v1.8+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a great way to get a cluster up quickly.
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) to interact with kubernetes.
- [`glooctl`](https://github.com/solo-io/glooctl) to interact with gloo.
- [`aws`](https://aws.amazon.com/cli/) (the aws cli) to create resources on AWS.


### Steps

1. Create a lambda function:

        # download the zipped function from github
        wget https://github.com/solo-io/gloo/raw/master/docs/getting_started/aws/helloWorld.zip

        # get the ARN for the "lambda_basic_execution" role
        ROLE_ARN=$(aws iam get-role --role-name lambda_basic_execution --query Role.Arn --output text)

        # create the function    
        aws lambda create-function \
        --region us-east-1 \
        --function-name helloWorld \
        --zip-file fileb://helloWorld.zip \
        --handler helloWorld.handler \
        --runtime nodejs6.10 \
        --role $ROLE_ARN


1. Install Gloo:

        kubectl apply \
                  -f https://raw.githubusercontent.com/solo-io/gloo/master/install/kube/install.yaml


1. Create a kubernetes secret with your AWS credentials:
        
        glooctl secret create aws --name aws-lambda-us-east-1  

   
1. Create an upstream for your AWS Account (for the us-east-1 region)

        cat <<EOF | glooctl upstream create -f -
        name: aws-lambda-us-east-1
        type: aws
        spec:
          region: us-east-1
          secret_ref: aws-lambda-us-east-1
        EOF
        
1. Verify that the upstream was created and the function was auto-discovered

        glooctl upstream get aws-lambda-us-east-1 -o yaml

        functions:
        - name: helloWorld:$LATEST
          spec:
            function_name: helloWorld
            qualifier: ""
        metadata:
          namespace: gloo-system
          resource_version: "5540"
        name: aws-lambda-us-east-1
        spec:
          region: us-east-1
          secret_ref: aws-lambda-us-east-1
        status:
          state: Accepted
        type: aws

1. Create a route to the function

        glooctl route create --sort \
           --upstream aws-lambda-us-east-1 \
           --function 'helloWorld:$LATEST' \
           --path-exact /hello 


1. Get the url of the ingress.  
If you installed kubernetes using minikube, you can use this command:

        GATEWAY_ADDR=$(kubectl get po -l gloo=ingress -n gloo-system -o 'jsonpath={.items[0].status.hostIP}'):$(kubectl get svc ingress -n gloo-system -o 'jsonpath={.spec.ports[?(@.name=="http")].nodePort}')
        
        export GATEWAY_URL=http://$GATEWAY_ADDR

 1. Try out the route using `curl`:
 
        curl $GATEWAY_URL/hello
 
        {"statusCode":200,"body":"AWS Lambda, brought to you by Gloo."}