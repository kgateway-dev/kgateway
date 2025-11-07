# EP-12721: E2e testing with Gateway API Versions


* Issue: [12721](https://github.com/kgateway-dev/kgateway/issues/12721)


## Background
The current e2e tests assume the latest supported experimental version of the `gateway.networking.k8s.io` APIs is installed. This will not always be the case in the environments in which kgateway is deployed. In order to validate functionality across a wider range of environments, we will allow testing with different versions of the Gateway API.

In addition to different semver designated versions of the API, there are two channels, `standard` and `experimental`


### Differences in API versions
* v0.3.0
  * TCPRoute, TLSRoute, and the unused UDPRoute added to experimental (not available in standard as of v1.4.0)
* v1.1.0
  * SessionPersistance for HTTPRoute rules added to experimental (not available in standard as of v1.4.0)
* v1.2.0
  * HTTPRoutes.spec.rules[].name added in experimental (promoted to standard in v1.4.0)
* v1.3.0
  * XListenerSets added to experimental (not available in standard as of v1.4.0, planned for v1.5.0)
  * CORS filters added to experimental (not available in standard as of v1.4.0)
* v1.4.0
  * BackendTLSPolicy promoted to v1 in standard and experimental. Previous v1alpha3 version is not supported.
  * HTTPRoutes.spec.rules[].name added to standard

The are a substantial number of tests that need to be modified to 


## Motivation
Better test coverage and understanding of how kgateway works with different Gateway API versions

## Goals
* E2E tests can be run locally or in CI with different versions (semver and channel) of the Gateway API
* Consistent approach to managing resources for different versions of the API

## Non-Goals
* Mass update of existing tests to use the BaseTestingSuite
  * Suites that don't use the BaseTestingSuite will continue to run all tests for any GatewayAPI version
  * Tests that need implement version dependent behavior will be migrated to BaseTestingSuite as needed
* Running tests in CRC/Openshift
* Updating application code to support earlier versions


## Implementation Details
### Determining the Gateway API version
The Gateway API CRDs contain two relevant annotations:
* `gateway.networking.k8s.io/bundle-version` - the API version, for example `gateway.networking.k8s.io/bundle-version: v1.2.0`
* `gateway.networking.k8s.io/channel` - the API channel, standard or experimental, for example `gateway.networking.k8s.io/channel: standard`

These annotations can be examined to determine the version. If the annotations are not present, this should be considered a fatal error.

### Test cases
The e2e tests are built up of [TestCases](https://github.com/kgateway-dev/kgateway/blob/2b04f3d1465257d0c449687922ea6e92603b822c/test/kubernetes/e2e/tests/base/base_suite.go#L33) that define the resources used for the tests.

In order to allow test cases to run conditionally based on the API version, we will add new fields, `MinGwApiVersion` and `MaxGwApiVersion` to the TestCase struct:

```
	// MinGwApiVersion specifies the minimum Gateway API version required per channel.
	// Map key is the channel (GwApiChannelStandard or GwApiChannelExperimental), value is the minimum version.
	// If the map is empty/nil, the test runs on any channel/version.
	// The test will only run if the Gateway API version is >= the specified minimum version.
	// For minimum requirements, if only experimental constraints exist, the test is considered experimental-only and will skip on standard channel.
	// Matching logic based on installed channel:
	//   - experimental: If experimental key exists, check version; otherwise run
	//   - standard: If standard key exists, check version; if only experimental exists, skip; otherwise runs on any standard version.
	MinGwApiVersion map[GwApiChannel]*GwApiVersion

	// MaxGwApiVersion specifies the maximum Gateway API version required per channel.
	// Map key is the channel (GwApiChannelStandard or GwApiChannelExperimental), value is the maximum version.
	// If the map is empty/nil, the test runs on any channel/version.
	// The test will only run if the Gateway API version is < the specified maximum version.
	// Maximum constraints are channel-specific - experimental constraints don't affect standard channel execution.
	// If the maximum version is less than the minimum version, the test will be skipped.
	MaxGwApiVersion map[GwApiChannel]*GwApiVersion
```

`MinGwApiChannel` is a typed string with the value of `experimental` or `standard`, and will define the minimum version of the API needed to run the test for the channel. If the current installation is now greater or equal to the required version, the test will be skipped. If no `MinGwApiChannel` value is defined, the test will run on any version of the API. The exception to this logic is if the standard channel is installed and the `MinGwApiVersion` for the TestCase only defines an experimental minimum version, for example:

```
    MinGwApiVersion: map[base.GwApiChannel]*semver.Version{
        base.GwApiChannelExperimental: base.GwApiV1_3_0,
    },
```

This will be interpreted as "the test needs to use features available in experimental API v1.4.0; these features are not yet available in the standard channel". In this case, the test will be skipped for all standard channel versions.

`GwApiVersion` is a wrapper around the underlying semver packages used, and was created in order to allow test suites to use semver types without having to know about the underlying implementation.


### Test Suites


#### SetupByVersion
A common pattern used in our e2e tests is to setup a Gateway and possibly other resources during suite setup and using them for every test. This pattern allows the tests to run faster, as time is not spent deploying and removing Gateways. In the [BaseTestingSuite](https://github.com/kgateway-dev/kgateway/blob/2b04f3d1465257d0c449687922ea6e92603b822c/test/kubernetes/e2e/tests/base/base_suite.go#L49C1-L66C2), these resources are defined by the [Setup](https://github.com/kgateway-dev/kgateway/blob/2b04f3d1465257d0c449687922ea6e92603b822c/test/kubernetes/e2e/tests/base/base_suite.go#L53) field

However, once we allow tests to run for different versions of the API, we are no longer in a "one configuration fits all" situation. For example, using ListenerSets requires `allowedListeners` to be defined on the Gateway, but this field will cause the resource to be rejected when using older versions of the API.

To accommodate this, we will add a new field `SetupByVersion` to the BaseTestingSuite:
```
	// SetupByVersion allows defining different setup configurations for different GW API versions and channels.
	// The outer map key is the channel (standard or experimental).
	// The inner map key is the minimum version, and the value is the TestCase to use.
	// The system will select the setup with the highest matching version for the current channel.
	// If no setups match, falls back to the Setup field for backward compatibility.
	// Example:
	//   SetupByVersion: map[GwApiChannel]map[*semver.Version]*TestCase{
	//     GwApiChannelExperimental: {
	//       GwApiV1_3_0: &setupExperimentalV1_3,
	//     },
	//     GwApiChannelStandard: {
	//       GwApiV1_3_0: &setupStandardV1_4,
	//     },
	//   }
	SetupByVersion map[GwApiChannel]map[*semver.Version]*TestCase
```

When choosing which setup to use, the suite will use the highest defined semver for the channel that is less than or equal to the current version, falling back to the existing `Setup` if there is no such version.

There are other data structures that could be used to store the setup information, but this approach was chosen because by making channel and version keys for the map, we guarantee that it will be unambiguous which setup to use.

#### MinGwApiVersion

`MinGwApiVersion` has also been added at the suite level to allow entire suites to be skipped.

This is used for the cases where all the tests in a suite require configuration not available in all Gw API versions, and it was introduced because test suites apply their setup before running (or skipping) the individual test cases. In these cases, the suite may run its setup with resources incompatible with the installed version of the Gw API, and we would not want to restore those resources.

### DevX
* This approach requires no changes for tests and suites that aren't version sensitive
* If a test needs to be skipped on certain versions, it can configured on the test case
* If a suite requires different setups/gateways based on version, once the setup is configured additional test cases just need to be congfigured with the versions they run on.


### Test Plan
Successful runs of a GitHub job across versions v1.2-1.4 in both channels.

Tests and suites will be adapted to older versions in 2 ways:
* If a test requires a feature (like XListenerSets or rule names in HTTPRoutes), those tests will be skipped.
* Some tests will fail because the suite setup or test resources have invalid config for a Gw API version, but the test itself does not. For example, a Gateway for the suite may be configured with `allowedListeners`, but only some tests use listenersets. In this case we will split the resources and use a combination of SetupForVersion and MinGwApiVersion to apply the appropriate config and run the appropriate tests for the Gw API version.

## Alternatives
Do not test other versions of the API.

## Open Questions
* Should we be able to set minimum version at the suite level? EG, for the listenerset suite, when we know that no tests in the suite will run?
