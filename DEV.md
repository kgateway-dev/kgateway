Architecture:

KGateway is a control plane for envoy based on the gateway-api. This means that we translate K8s objects like Gateways, HttpRoutes, Service, EndpointSlices and User policy into envoy configuration.

Our goals with the architecture of the project are to make it scalable and extendable.

To make the project scalable, its importnat to keep the computation minimal when changes occur. For example, when a pod changes, or a policy is updated, we don't do the minimum amount of computation to update the envoy configuration.

With extendability, we KGateway to be the basis on-top of which users can easily add their own custom logic. to that end we have a plugin system that allows users to add their own custom logic to the control plane in a way that's opaque to the core code.


Going down further, to enable these goals we use KRT based system. KRT gives us a few advantages:
- The ability to complement controllers of custom Intermediate representation (henceforth IR).
- Automatically track object dependencies and changes and only invoke logic that depends on the object that changed.

# CRD Journey
How does a user CRD make it into envoy?

We have 3 main translation lifecycles: Routes & Listeners, Clusters and Endpoints.

Let's focus on the first one - Routes and Listeners, as this is where the majority of the logic is.

Envoy Routes & Listeners translate from Gateways, HttpRoutes, and user policies (i.e. RoutePolicy, ListenerPolicy, etc).

## Policies
The first step is to convert each user policy into an IR form. This is done by creating a collection of these objects from k8s, and transforming the collection to an IR representation.

For policies, this is pluggable. Plugins can Contribute a policy to kgateway. Contributing a policy means that we add a policy collection to  kgateway. It's the users plugin responsibility to convert the policy CRD to the IR form. Ideally, the IR should look as close as possible to the envoy configuration, so this translation only happens when the policy CRD changes.

You can see in the Plugin interface a field called `ContributesPolicies` which is a map of GK -> `PolicyPlugin`.
The policy plugin contains a bunch of fields, but for out discussion we'll focus on these two:

```go
type PolicyPlugin struct {
	Policies       krt.Collection[ir.PolicyWrapper]
	NewGatewayTranslationPass func(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass
    // ... other fields
}
```
Policies is a the collection of policies that the plugin contributes. The plugin is responsible to create
this collection, usually by started with a CRD collection, and then translating to a `ir.PolicyWrapper` struct.

Lets look at the important fields in the PolicyWrapper:

```go
type PolicyWrapper struct {
	// The Group, Kind Name, Namespace to the original policy object
	ObjectSource `json:",inline"`
	// The IR of the policy objects. ideally with structural errors removed.
	// Opaque to us other than metadata.
	PolicyIR PolicyIR
	// Where to attach the policy. This usually comes from the policy CRD.
	TargetRefs []PolicyTargetRef
}
```

The system will make use of the traget refs to attach the policy IR to Listners and HttpRoutes. You will then 
get access to the IR during translation (more on that later).

The second field, `NewGatewayTranslationPass` allocates a new translation state for the
gateway/route translation. This function will be invoked during the Translation to xDS phase, so will expand on it later.

## HttpRotues and Gateways

HttpRoutes and Gateways are handled by KGateway. Kgateway builds an IR for HttpRoutes and Gateways, that looks very similar to 
the original object, but in additional has an `AttachedPolicies` struct that contains all the policies that are attached to the object.

KGateway uses the `TargetRefs` field in the PolicyWrapper to opaquely attach the policy to the HttpRoute or Gateway.

## Translation to xDS

When we reach this phase, we already ahve the Policy -> IR translation done; and we have all the HttpRoutes and Gateways in IR form with the policies attached to them.

In the begining of the transaltion to xDS, we take all the contributed policies and allocate a `NewGatewayTranslationPass` for each of them. This will hold the state for the length of the translation.

This allows us for example translate a route, and in response to that hold state that tells us to add an http filter.

KGateway handles merging of httproutes per gw-api spec. When it translates GW-api route rules to 
envoy routes, it reads the `AttachedPolicies` and calls the appropriate function in the `ProxyTranslationPass` and passes in 
the attached policy IR. This let's the policy plugin code the modify the route or listener as needed, based on the policy IR.