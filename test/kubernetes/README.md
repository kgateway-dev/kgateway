# Kubernetes Tests

## Testing Philosophy
Our goal for Kubernetes tests is to mirror the behavior that users take on the Gloo Gateway product as much as possible.

## End-To-End Testing
### Historical Challenges
_We document the historical challenges we have experienced with writing and managing end-to-end tests as a way of avoiding making the same mistakes:_

#### Not Representative of User Behaviors
- Tests were too aware of implementation details, and would have custom behaviors built into them (ie. deleting Proxy CRs after deleting the sourcing VirtualService and Gateway).
- Actions were taken on the cluster that users wouldn't typically do. For example, we would patch the Settings resource via a Go method. While patching this resource is an available operation to users, are more common practice would be updating the manifest in a CI/CD system and rolling it out to your cluster via Helm.
- Code was written to mirror user actions, but that code wasn't then made available in our CLI. This meant that we built useful tooling, but didn't empower users of the product to benefit.

#### Confusing Resource Management
- There were deeply nested structures for configuring resources (BeforeEach). This meant that it was challenging to identify which resources would be available for a given test.
- It was easy to forget to clean up resources in a given test. We re-use a Kubernetes Cluster for a series of tests, so leaving behind resources led to inconsistent behaviors and test flakes.
- Our mechanism to apply resources to a cluster was built around the concept of an [ApiSnapshot](/projects/gloo/pkg/api/v1/gloosnapshot/api_snapshot.sk.go). This meant that when we wanted to apply resources not in that Snapshot, we would not do so consistently.

#### Tightly Coupled Code Structure
- There was a 1:1:1 relationship between: GitHub actions (infrastructure), test suites, installation values of Gloo Gateway. This meant that as you scaled the number of different Helm values you wanted to run tests against, you had to set up new infrastructure. This was time-consuming and expensive, and caused toil in adding new tests cases, so they would not be added.
- Test cases were defined in the test file, which meant that if you wanted to run that same test under a different set of circumstances (different install values, open source v enterprise) you had to re-define the test.
- There was not a clear separation of concerns between infrastructure setup, installation of Gloo Gateway, application of resources, assertions of behaviors.The impact was that if a developer wanted to perform partial actions in a test, they had to use lots of environment variables to control behavior. These variables were documented, but still challenging to use.

#### Challenging Developer Maintenance
- There was a set of utilities that were distributed across the codebase, and sometimes custom written for individual tests. This meant that tests were written using different utilities and thus behaved differently.
- When triaging a test, it was challenging to run the same test repeatedly. Re-running a test repeatedly would cause the entire installation and uninstallation to be run each time. This meant that investigating flaky behaviors was extremely time-consuming.
- Tests were grouped in large files, not necessarily by their functionality, so if you wanted to know _"do we test behavior X"_, you had to know where to look to find the answer.
- After reproducing a behavior in a cluster, it was challenging to convert these resources (manifests) into Go structs to be used within the test structure. Again, this led to toil that either resulted in wasted time writing tests, or a rationale to not add a test.

### Framework
We've learned from these challenges and have introduced a framework that we believe addresses the concerns. This framework allows us to write expressive end-to-end tests that reflect the experiences of users of the product.

_For more details on the end-to-end framework, see the [e2e](./e2e) package._