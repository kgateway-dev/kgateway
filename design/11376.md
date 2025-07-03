# EP-11376: Modular Deployer
 
Status:
 
Issue: https://github.com/kgateway-dev/kgateway/issues/11376
 
## Background
Currently kgateway uses a Deployer to automate deploying of gateways and inference pools. Its implementation is tightly coupled with internal representation of gateways, gateway parameters, associated helm chart values, and the inference pool extension is treated as a special case while largely relying abstractions originally developed for gateways.
The Deployer pattern and its implementation could be reused for handling of deployments of components other than gateways (and extension pools) by creating a common interface for rendering of charts and pushing gateway-specific chart rendering code into a dedicated concrete implementation of that interface.

## Goals
1. Decouple Deployer from implementation details of gateway and inference pool helm chart rendering
2. Make Deployer a public sub-module so it can be reused
3. Support for arbitrary GatewayParameters extensions
4. Make GatewayParameter merging logic available for reuse

## Non-Goals
1. Re-use of kgateway controller and inference extensions controller
2. Support for handling of multiple charts per single Deployer instance
3. Optimization(s) of helm hart rendering

## Alternatives
* The Deployer can be further broken up into Applier & Rendered interfaces and their default implementations. 

## Implementation Details

#### Current State

![current implementation of Deployer](resources/deployer-current-implementation.png "current implementation of Deployer")
1. Deployer exposes a set of functions to render (GetObjsToDeploy, GetEndpointPickerObjs) and deploy (DeployObjs) for gateways and inference pools. The deployer uses hard-coded logic and configuration in Inputs (4) to automatically configure itself for deploying of gateways or inference pools. Rendering of values and helm charts for gateways and inference pools is an implementation detail not exposed outside of internal/deployer package.
2. helmConfig is used to store helm values generated during GetObjsToDeploy and GetEndpointPickerObjs calls and is not accessible outside of internal/deployer package.
3. Chart is a reference to a helm chart (helm module) 
4. Inputs is a set of options used to configure the deployer, such as control plane xds config, inference extension config, image repository config, etc. These are used during rendering of helm charts.
5, 6. controllers use Deployer to render kgateway and inference extension charts and then sync the changes to the k8s cluster.

#### Proposed Changes

![proposed implementation of Deployer](resources/deployer-proposed-changes.png "proposed implementation of Deployer")
1. Move [Deployer into pkg/deployer package](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-f3c8137f0e1c7fd0ea790cd1766fded900c4c1466962827294d3efac0e85f840R57); reduce its interface to GetObjsToDeploy (rendering of charts) and DeployObjs (syncing objects to the k8s cluster).
2. Move [Inputs into pkg/deployer package](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-f3c8137f0e1c7fd0ea790cd1766fded900c4c1466962827294d3efac0e85f840R40); it's a direct dependency of Deployer and is required to create an instance of one.
3. Chart is a reference to a helm chart (helm module). Charts are no longer loaded inside Deployer factory function, instead it becomes a responsibility of the controllerBuilder.
4. [HelmValuesGenerator](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-9d7c35479e65bac7fd480cd082cb6c7d34679d56675ab6c79df66e606bb3cd0eR15) is a common interface for generating of helm values used with helm charts. Implementations for handling of [gateway parameters](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-f985bb8743470d3a6c7d2a3c524fae007e8912e37a001e5b3dbadb35c5379fa2R34) (7) and [inference extensions](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-16d2eb16223cdcf1bae9a6cdfadf95052bb0f6ae4aa54dde575795aeac5ac713R12) (8) reside in internal/deployer package.
5. [HelmConfig](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-03ea6362b8b7909144dd2a063e851ccc3de0602397c1ea0ac771a835e22091a9R10) is now a public struct that is used to store helm values for gateway and inference extension charts. This is done to support reuse of gateway parameters helm values generation.
6. [pkg/deployer/GatewayParameters](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-9d7c35479e65bac7fd480cd082cb6c7d34679d56675ab6c79df66e606bb3cd0eR53) is a module that makes kgateway default config parameters available for reuse
7, 8. Helm values renderers, [instantiated by controllerBuilder and then injected](https://github.com/kgateway-dev/kgateway/pull/11377/files#diff-c04d0ac99726d6a18164b34a2ec4a8bacbee9667ec7ee3260b5bb930f545eb42R202) into a Deployer instance.
9, 10. controllers use Deployer to render kgateway and inference extension charts and then sync the changes to the k8s cluster.

## PoC Implementation
https://github.com/kgateway-dev/kgateway/pull/11377