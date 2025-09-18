# EP-12224: Correct Status on Gateway/Listeners

* Issue: https://github.com/kgateway-dev/kgateway/issues/12224

## Background
During IR translation we are dropping listeners that are translated as nil or do not have a filter chain. There are two current issues:
* When a listener is dropped because it is a TCP gateway with no routes, that is not captured in any listener conditions
* When a listener is dropped because it is invalid for any reason, no Gateway conditions are updated

Known conditions that cause a listener to be dropped:
* TCP listeners with no routes
* TLS listeners with no routes
* HTTPS routes with invalid secrets

### Docs
* [ListenerConditionAccepted](https://github.com/kubernetes-sigs/gateway-api/blob/release-1.3/apis/v1/gateway_types.go#L1156-L1183)
* [ListenerConditionProgrammed](https://github.com/kubernetes-sigs/gateway-api/blob/release-1.3/apis/v1/gateway_types.go#L1261-L1286)
* [GatewayConditionAccepted](https://github.com/kubernetes-sigs/gateway-api/blob/release-1.3/apis/v1/gateway_types.go#L975-L999)
* [GatewayConditionProgrammed](https://github.com/kubernetes-sigs/gateway-api/blob/release-1.3/apis/v1/gateway_types.go#L898-L926)

## Motivation

To have Gateway statuses and Listener subresource statuses be accurate and consistent.

## Goals

* To have Gateway statuses and Listener subresources statuses be accurate and consistent.
* To have a documented policy of where Listener statuses should be written

## Non-Goals
* Consolidating Listener status types other than Programmed, though the solution implemented should be easily extensible.

## Implementation Details
* It will be the responsibilty of the per-Listener translation loop to properly set status conditions on the Listener
  * Fix the existing gap in reporting for TCP gateways with no routes.
* The Gateway conditions will be updated by the reports package.

From the docs:
GatewayConditionProgrammed
```
	// This condition indicates whether a Gateway has generated some
	// configuration that is assumed to be ready soon in the underlying data
	// plane.
	//
	// It is a positive-polarity summary condition, and so should always be
	// present on the resource with ObservedGeneration set.
	//
	// It should be set to Unknown if the controller performs updates to the
	// status before it has all the information it needs to be able to determine
	// if the condition is true.
	//
	// Possible reasons for this condition to be True are:
	//
	// * "Programmed"
	//
	// Possible reasons for this condition to be False are:
	//
	// * "Invalid"
	// * "Pending"
	// * "NoResources"
	// * "AddressNotAssigned"
	//
```

GatewayConditionAccepted
```
	// This condition is true when the controller managing the Gateway is
	// syntactically and semantically valid enough to produce some configuration
	// in the underlying data plane. This does not indicate whether or not the
	// configuration has been propagated to the data plane.
	//
	// Possible reasons for this condition to be True are:
	//
	// * "Accepted"
	// * "ListenersNotValid"
	//
	// Possible reasons for this condition to be False are:
	//
	// * "Invalid"
	// * "InvalidParameters"
	// * "NotReconciled"
	// * "UnsupportedAddress"
	// * "ListenersNotValid"
```

Based on these guidelines, when there are invalid listeners the Gateway conditions should be updated:
* GatewayConditionProgrammed - should not be modified from current behavior of "Status: true" and "Reason: Programmed"
* GatewayConditionAccepted - will be reported as "Status: True", "Reason: ListenersNotValid", with a Message that reflects the state of the listeners.


Translation unit tests with golden files.

## Alternatives

## Open Questions

