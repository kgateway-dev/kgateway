<!--
```
<<[UNRESOLVED optional short context or usernames ]>>
The actual design of the Upstream/Backend type still needs to be fleshed out.
As it stands, we have a tentative design plan as outlined in the
"Differences in K8s Gateway API" section, but this should probably be formalized
in a standalone EP/design doc.
@jenshu
@timflannagan
<<[/UNRESOLVED]>>
```
-->
# EP-10611: Rename Upstream type to Backend

* Issue: https://github.com/kgateway-dev/kgateway/issues/10611

## Background 

Kgateway's primary focus is providing an exceptional vendor-neutral, OSS API Gateway based on the Kubernetes Gateway API.
To reach this goal, a main area of focus has been on removing the legacy Gloo Gateway types as well as reshaping Gloo Gateway APIs that are either redundant or do not follow the semantics and idioms of the K8s Gateway API.

One such API that needs to be revisited is the carryover of the `Upstream.gloo.solo.io` type.

This resource provides two different “layers” of config related to expressing a backend for routing:
* Backend type 
* Backend policy

### Backend type

A backend type allows a user to define a routing destination.
Some examples of backend types are: AWS Lambda functions, GCP Cloud Run services, static (i.e. external) hosts, K8s Services, etc.

AWS Lambdas and GCP Cloud Run services are examples of backends that have no native representation in Kubernetes, thus a standalone resource to represent them makes complete sense.

Static/external hosts theoretically have a way to be expressed via K8s as `Service` of `ExternalName` type, although this usage is not supported in Gloo Gateway (and now, kgateway) mostly due to lack of user demand.
Additionally, there are some caveats with ExternalName services that may discourage use if there is a viable alternative. More info is available on the [ExternalName documentation](https://kubernetes.io/docs/concepts/services-networking/service/#externalname).
Given the above, for Gloo Gateway (and now, kgateway) using the `Upstream` resource to define external hosts also makes sense.

Other projects have similar concepts, such as [Istio's ServiceEntry type](https://istio.io/latest/docs/concepts/traffic-management/#service-entries).

However, core K8s `Services` already express an in-cluster traffic destination and are a valid backend type, which makes it questionable why Gloo Gateway `Upstreams` supported `Services` as a backend type.
The reason for this redundancy is related to the second "layer" of config provided by `Upstreams`, which is backend policy.

### Backend policy

Backend policy is a broad concept that loosely describes behavior related to interacting with the backend, regardless of the _type_ of backend. Note that some policy is only applicable to some backend types, but in general this guidance applies.

For example, users may want to connect to a backend using mTLS and need a way to express this as a policy.
In Gloo Gateway the `Upstream` type is also where users will configure this policy.
In the mTLS example, users would provide a `Secret` which contains the certificates and keys needed to perform the mTLS handshake when establishing a connection to the desired backend.

## Differences in K8s Gateway API

The `Upstream` name and concept are well-established for GG users but the current API does not completely fit the semantics and idioms of the K8s Gateway API.

The first "layer" of defining various backend types that do not have a native K8s representation is still relevant & important within K8s Gateway API, with the exception of not needing to support native `Services`.
In fact, defining a custom type to reference as a backend is the [primary extension point](https://gateway-api.sigs.k8s.io/concepts/api-overview/#extension-points) for supporting non-K8s backends.

However, for the second "layer" of backend policy, the K8s Gateway API is moving in a different direction.
Policy should be expressed as a separate resource that can be "attached" to the object to augment interaction with the backend.
This also enables policy to be defined without needing to modify the `spec` of the backend type.
See the [`BackendTLSPolicy` type](https://gateway-api.sigs.k8s.io/api-types/backendtlspolicy/) as a prime example.

## Kgateway

Kgateway currently has a backend type, `Upstream.kgateway.dev`, of which the name is simply a holdover from the original GG `Upstream`.

However, as outlined above, we will want this type to progress towards the K8s Gateway API direction.
This means the kgateway type will have different semantics from `Upstream.gloo.solo.io` and will continue to diverge.

## Motivation

### Goals

* Agreement on incompatibilities with the `Upstreams.gloo.solo.io` type and K8s Gateway API semantics
* Agreement on a new `kind` name to represent backend destinations

### Non-Goals

* Agreement on the specific design or API shape of the `Backend` type

## Proposal

**Rename the `Upstream.kgateway.dev` type to `Backend.kgateway.dev`.**

This will help distinguish the fact that these are different resources and have different semantics.
This should reduce confusion for existing Gloo Gateway users moving to kgateway as well anybody that finds older Gloo Gateway documentation, examples, etc.

Additionally, the `Backend` name follows the K8s Gateway API, as "backend" is the accepted term for referring to a routing destination.

Notably, the core routing primitives all refer to `backendRefs` to define routing rules.
For example, `HTTPRoutes` will refer to https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPBackendRef

Another data point is the aforementioned [BackendTLSPolicy](https://gateway-api.sigs.k8s.io/api-types/backendtlspolicy/), which provides TLS policy for backends.

An additional minor benefit to renaming this type is that users migrating from Gloo Gateway to kgateway by having both installed one the same cluster will not have clashing `upstream` types, which can cause poor UX when interacting with the cluster via CLI or any tooling that uses the non-qualified name.

## Alternatives

* Upstream
* Destination
* KUpstream
