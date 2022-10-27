
When creating a new resource Group extend the Validator.go method`ModificationIsSupported()` and `DeletionIsSupported()`.

We will also have to update the Webhook Validator helm values as well in `install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml`