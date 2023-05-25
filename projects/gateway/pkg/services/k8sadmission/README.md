# Kubernetes Admission Services

Kubernetes provides [dynamic admission control](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/), pieces of code that intercept requests to the Kubernetes API server, before the resource is persisted, but after the request has been authenticated and authorized.

There are two types of admission controllers which can be configured at runtime, and run as webhooks:
- *MutatingAdmissionWebhook* (invoked first, and can modify objects sent to the API server to enforce custom defaults)
- *ValidatingAdmissionWebhook* (invoked second, and can reject requests to enforce custom policies)

## MutatingAdmissionWebhook
Gloo does not currently configure a ValidatingAdmissionWebhook

## ValidatingAdmissionWebhook
Gloo leverages the ValidatingAdmissionWebhook to validate proposed changes to custom resources (i.e. a VirtualService) before they are persisted in etcd. Where the [structural schemas on CRDs](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#specifying-a-structural-schema) provide syntactic validation, the validating webhook provides semantic validation.

### Execution
#### Which resources are subject to the ValidatingAdmissionWebhook?
The ValidatingWebhookConfiguration is part of the Kubernetes API, and configured through a [Helm template](https://github.com/solo-io/gloo/blob/main/install/helm/gloo/templates/5-gateway-validation-webhook-configuration.yaml). Based on the webhook rules, only the API groups/resources that match the rules will be subject to validation

#### Where is the Webhook entrypoint defined?
The ValidatingWebhookConfiguration defines the `name` and `path` to the service which handles validation requests. In Gloo, this is the Gloo Service, at the `/validation` endpoint.

The [ValidatingAdmissionWebhook contstructor](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/services/k8sadmission/validating_admission_webhook.go#L141) returns the `http.Server` which can handle these requests.

#### How is communication with the Webhook secured?
To verify the TLS connection with the webhook, a PEM-encoded CA bundle for validating the webhook's server certificate is provided in the webhook configuration.

The `CaBundle` can be defined in the following ways:
1. Manually
2. Using the [Gloo Edge Certgen Job](https://github.com/solo-io/gloo/tree/main/jobs/certgen/cmd)
3. Using the [Cert-Manager CA Injector](https://cert-manager.io/docs/concepts/ca-injector/)

#### When the ValidatingAdmissionWebhook is invoked, how does Gloo perform validation?
Gloo performs a robust set of validation during translation of resources, so that it does not push invalid configuration to the data plane. The warnings and errors that are encountered during translation, are aggregated on [ResourceReports](https://github.com/solo-io/solo-kit/blob/33fda1f5c53cd3c91298760d2f275f6b834a424d/pkg/api/v2/reporter/reporter.go#L24).

Instead of re-defining this same set of validation code in our webhook, we re-use the existing code by performing the following steps:
1. An HTTP request is [received by the server](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/services/k8sadmission/validating_admission_webhook.go#L192)
2. The request is unmarshalled, and then [validated by the webhook](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/services/k8sadmission/validating_admission_webhook.go#L383)
3. The webhook uses a [configurable Validator](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/services/k8sadmission/validating_admission_webhook.go#L160) to perform this logic. The validator implementation is [defined here](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/validation/validator.go#L99)
4. The validator first runs [translation of Gateway resources](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/validation/validator.go#L288) and rejects the proposal if warnings/errors were encountered during translation
5. The validator then runs [gloo translation with the generated proxy](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/validation/validator.go#L305), and rejects the proposal if warnings/errors were encountered during translation
6. If both Gateway translation and Gloo translation do not produce errors/warnings, the [resource is accepted](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gateway/pkg/validation/validator.go#L349)

### Configuration
#### Where is the webhook configuration defined?
Webhook configuration is defined on the [Gloo Settings resource](https://github.com/solo-io/gloo/blob/a3430da820bd39a8b0940025c1040e33eeb7d8f8/projects/gloo/api/v1/settings.proto#L605)

### Debugging
#### What if requests aren't received by the webhook?
1. Turn on [debug logs](https://docs.solo.io/gloo-edge/latest/operations/debugging_gloo/#changing-logging-levels-and-more) and confirm that requests are not being processed by the validation webhook.
2. Change the [failure policy](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#failure-policy) to Fail, so that if the webhook cannot be called, the admission request will be rejected.
3. Retry a change that would invoke the webhook, and examine the logs for pointers.

Commonly this occurs when the certificate has expired and needs to be regenerated.