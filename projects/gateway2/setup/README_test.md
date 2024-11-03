Envtests for krt/ggv2

Add a `.yaml` in the test folder.
The first time your run the test, an xds `-out.yaml` file will be created in the same folder.

From here on, it will compare the xds outputs of the `scenario.yaml` of the test with the `-out.yaml` file.