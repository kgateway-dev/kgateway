Envtests for krt/ggv2

Add a `.yaml` in the test folder.
The first time your run the test, an xds `-out.yaml` file will be created in the same folder.

From here on, it will compare the xds outputs of the `scenario.yaml` of the test with the `-out.yaml` file.

It is assumed that the scenario yaml has gateway named `http-gw-for-test` and a pod named `gateway`.
The test will rename the gateway, so that the tests can run in parallel. Make sure that other resources
in the scenario yamls are unique.

The test will apply the resources in the yaml file, ask for an xDS snapshot, and finally compare the snapshot with the `-out.yaml` file.