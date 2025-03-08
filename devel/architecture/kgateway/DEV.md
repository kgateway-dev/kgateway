# Architecture
Kgateway is a control plane for envoy based on the gateway-api. This means that we translate K8s objects like Gateways, HttpRoutes, Service, EndpointSlices and User policy into envoy configuration.

Our goals with the architecture of the project are to make it scalable and extendable.

To make the project scalable, it's important to keep the computation minimal when changes occur. For example, when a pod changes, or a policy is updated, we do the minimum amount of computation to update the envoy configuration.

With extendability, Kgateway is the basis on-top of which users can easily add their own custom logic. to that end we have a plugin system that allows users to add their own custom logic to the control plane in a way that's opaque to the core code.


Going down further, to enable these goals we use [KRT](https://github.com/istio/istio/tree/master/pkg/kube/krt#krt-kubernetes-declarative-controller-runtime) based system. KRT gives us a few advantages:
- The ability to complement controllers of custom Intermediate Representation (henceforth IR).
- Automatically track object dependencies and changes and only invoke logic that depends on the object that changed.

# CRD Lifecycle

How does a user CRD make it into envoy?

We have 3 main translation lifecycles: Routes & Listeners, Clusters and Endpoints.

Let's focus on the first one - Routes and Listeners, as this is where the majority of the logic is.

Envoy Routes & Listeners translate from Gateways, HttpRoutes, and user policies (i.e. RoutePolicy, ListenerPolicy, etc).

## Policies (Contributed by Plugins)

To add a policy to KGateway, it needs to be contributed through a **plugin**. Each plugin acts as an independent Kubernetes controller that provides Kgateway with a **KRT collection of IR policies** (this is done by translating the policy CRD to a policy IR). The second important thing a plugin provides is a function that allocates a **translation pass** for the gateway translation phase. This will be used when doing xDS translation. More on that later.

To add a policy to Kgateway we need to 'contribute' it as a plugin. You can think of a plugin as an
independent k8s controller that translates a CRD to an krt-collection of policies in IR form Kgateway can understand (it's called `ir.PolicyWrapper`).
A plugin lifecycle is the length of the program - it doesn't reset on each translation (we will explain later where we hold translation state).

When a plugin can contribute a policy to Kgateway, Kgateway uses policy collection to perform **policy attachment** - this is the process of assigning policies to the object they impact (like HttpRoutes) based on their targetRefs.

You can see in the Plugin interface a field called `ContributesPolicies` which is a map of GK -> `PolicyPlugin`.
The policy plugin contains a bunch of fields, but for our discussion we'll focus on these two:

```go
type PolicyPlugin struct {
	Policies       krt.Collection[ir.PolicyWrapper]
	NewGatewayTranslationPass func(ctx context.Context, tctx ir.GwTranslationCtx) ir.ProxyTranslationPass
    // ... other fields
}
```
Policies is a the collection of policies that the plugin contributes. The plugin is responsible to create
this collection, usually by starting with a CRD collection, and then translating to a `ir.PolicyWrapper` struct.

Lets look at the important fields in the PolicyWrapper:

```go
type PolicyWrapper struct {
	// The Group, Kind Name, Namespace to the original policy object
	ObjectSource `json:",inline"`
	// The IR of the policy objects. Ideally with structural errors either removed so it can be applied to envoy, or retained and returned in the translation pass (this depends on the defined fallback behavior).
	// Opaque to us other than metadata.
	PolicyIR PolicyIR
	// Where to attach the policy. This usually comes from the policy CRD.
	TargetRefs []PolicyTargetRef
}
```

The system will make use of the targetRefs to attach the policy IR to Listners and HttpRoutes. You will then 
get access to the IR during translation (more on that later).

When translating a Policy to a PolicyIR, you'll want to take the translation **as close as you can** to envoy's config structures. For example, if your policy will result in an envoy
PerFilterConfig on a route, the PolicyIR should contain the proto.Any that will be applied there. Doing most of the policy translation at this phase as advantages:
- Policy will only re-translate when the policy CRD changes
- We can expose errors in translation on the policy CRD status as soon as the policy is written (no need to depend on later translation phases).

The second field, `NewGatewayTranslationPass` allocates a new translation state for the
gateway/route translation. This function will be invoked during the Translation to xDS phase, so will expand on it later.

## HttpRotues and Gateways

HttpRoutes and Gateways are handled by Kgateway. Kgateway builds an IR for HttpRoutes and Gateways, that looks very similar to 
the original object, but in additional has an `AttachedPolicies` struct that contains all the policies that are attached to the object.

Kgateway uses the `TargetRefs` field in the PolicyWrapper (and extensionRefs in the Route objects) to opaquely attach the policy to the HttpRoute or Gateway.

## Translation to xDS

When we reach this phase, we already have the Policy -> IR translation done; and we have all the HttpRoutes and Gateways in IR form with the policies attached to them.

The translation to xDS has 2 phases.
- The first phase, aggregating all the httproutes for a single gateway into a single gateway IR object with all the policies and routes inside it. This resolves delegation, and merges http routes based on the Gateway Api spec. The phases uses KRT to track the dependencies of the routes and gateways (like for example, ssl certs). recall that by the time we get here, policy attachment was already performed (i.e. policies will already be in the AttachedPolicies struct).
- The second phase, translates the gateway IR to envoy configuration. It does not use KRT, as all the dependencies are resolved in the first phase, and everything needed for translation is in the gateway IR. At this phase the **policy plugins** are invoked.

Having these 2 phases has these advantages:
- The second phase is simple and fast, as all the dependencies are resolved in the previous phases.
- We can create alternative translators for the gateway IR, for example, to translate a waypoint.


In the begining of the transaltion of the gateway IR to xDS, we take all the contributed policies and allocate a `NewGatewayTranslationPass` for each of them. This will hold the state for the duration of the translation. 

This allows us for example translate a route, and in response to that hold state that tells us to add an http filter.

When it translates GW-api route rules to envoy routes, it reads the `AttachedPolicies` and calls the appropriate function in the `ProxyTranslationPass` and passes in 
the attached policy IR. This let's the policy plugin code the modify the route or listener as needed, based on the policy IR (add filters, add filter route config, etc..).
Then policy plugins are invoked with the attached policy IR, and can modify the envoy protbufs as needed 

# Diagram of the translation lifecycle

![](translation.svg)