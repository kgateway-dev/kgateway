Envtests for krt/kgateway

Add a `.yaml` in the test folder.
The first time your run the test, an xds `-out.yaml` file will be created in the same folder.
Note:
- The test will fail in this case
- The kubernetes service endpoint will not be written, as it has a different port every run.

From here on, it will compare the xds outputs of the `scenario.yaml` of the test with the `-out.yaml` file.

It is assumed that the scenario yaml has gateway named `http-gw-for-test` and a pod named `gateway`.
The test will rename the gateway, so that the tests can run in parallel. Make sure that other resources
in the scenario yamls are unique (though currently tests won't run in parallel).

The test will apply the resources in the yaml file, ask for an xDS snapshot, and finally compare the snapshot with the `-out.yaml` file.

## How to run

From the `kgateway/internal/kgateway` directory run:

```shell
make install-go-tools
```

Then run the tests in the setup directory:
```yaml
go test -v ./setup/
```

Test resources:
- testdata/setup_yaml/setup.yaml: Adds the GatewayClass and GatewayParameters
- testdata/setup_yaml/pods.yaml: Adds the shared pods and nodes
- testdata/istio_crds_setup/crds.yaml: Adds istio CRDs

Test setups:

- `standard`: `setup/standard` uses the standard kgateway setup
- `istio_mtls`: `setup/istio_mtls` uses the standard kgateway setup with Istio auto mTLS enabled
- `autodns`: `setup/autodns` uses the standard kgateway setup with auto DNS enabled
- `istio_service_entry`: `setup/istio_service_entry` uses the standard kgateway setup with Istio service entry integration enabled
- `istio_destination_rule`: `setup/istio_destination_rule` uses the standard kgateway setup with Istio destination rule integration enabled
- `inference_api`: `setup/inference_api` uses the standard kgateway setup with Inference API enabled