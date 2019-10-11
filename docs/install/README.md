After a new version of the docs are created you must deploy the manifest and add a route to the service corresponding to the new version.

A sample virtual service is provided in `sample_virtualservice.yaml`.

Note the path is the docs version.


## TODO/Questions
- automate the deployment and route creation
- decide how granular to version the docs
  - now that the docs are in the Gloo repo, the only way to edit the docs is to release a new version of Gloo.
  - we should probably use Envoy's example, and host docs for each major release as well as a "latest"
