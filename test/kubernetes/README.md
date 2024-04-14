# Kubernetes Tests

## Testing Philosophy


## Test Utilities
### Assertions
_For more details on the assertions, see the [assertions](./testutils/assertions) package._

### Operations
_For more details on the operations, see the [operations](./testutils/operations) package._




## End-To-End Testing

### Historical Challenges
_We document the historical challenges we have experienced with writing and managing end-to-end tests as a way of avoiding making the same mistakes_

Below are some challenges we have had while writing end-to-end tests:

- Nested structures for configuring resources
- Cleanup of resources could easily be forgotten and lead to 
- Tests were too aware of implementation details
- Actions were taken on cluster that users couldn't do
- Tooling to perform actions and debug cluster was custom for tests, and not useful for end users
- Distributed set of utilities that were easy to forget
- Inconsistent mechanisms for configuring resources and asserting state led to test flakes
- Challenging to run a test over and over (to triage flakes)
- Challenging to convert between local manifests and test structure. So if you reproduced a behavior, it took extra time to convert that in to a test
- Challenging to configure resources that weren't in the ApiSnapshot
- An entire suite was associated with a single installation of Gloo Gateway. This meant that everytime we want to test a new set in install values, we would often spin off a new suite, and a new suite meant a new cluster

### Framework
We attempt to learn from those 
