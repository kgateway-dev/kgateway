# Adding fields to Policy CRDs

This page covers the end-to-end workflow for adding a field to an existing Policy CRD
(TrafficPolicy, HTTPListenerPolicy, and friends). For adding a brand new CRD, and for the
field-level API conventions, see the [API README](/api/README.md).

1. Add the field to the appropriate `Spec` struct in the CRD Go type under [api/v1alpha1](/api/v1alpha1).
2. Add validation markers as needed (e.g. `+kubebuilder:validation:MinLength=1`, `+optional`).
   See the [API guidelines](/api/README.md#api-guidelines).
3. Run `make go-generate-apis` to regenerate code, then `make fmt-changed`.
4. Update the IR struct in the plugin package
   (`pkg/kgateway/extensions2/plugins/<plugin_name>/`) to include the new field.

   Translate as close to the Envoy protos as possible **here**, not in the translation pass —
   the translation pass should stay very lightweight. Doing the work in the IR keeps logic out
   of the final translation and lets us report better status back to the user on errors.
5. Add yaml test cases in
   [pkg/kgateway/translator/gateway/gateway_translator_test.go](/pkg/kgateway/translator/gateway/gateway_translator_test.go).

   The yaml inputs go in `pkg/kgateway/translator/gateway/testutils/inputs/`.
   **Do not write the outputs by hand.** Run the test with `REFRESH_GOLDEN=true` and it will
   generate them in `pkg/kgateway/translator/gateway/testutils/outputs/` for you:

   ```bash
   REFRESH_GOLDEN=true go test -timeout 30s \
     -run '^TestBasic$/^ListenerPolicy_with_proxy_protocol_on_HTTPS_listener$' \
     github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/gateway
   ```

   Once the outputs are generated, inspect them to confirm they contain the changes you expect.
6. For non-trivial changes, also add unit tests.
7. Consider adding an E2E test using the framework in [test/e2e](/test/e2e).
   [test/e2e/features/cors/suite.go](/test/e2e/features/cors/suite.go) is a good example.
    - Prefer `base.NewBaseTestingSuite` as the base suite — it provides many useful utilities.
    - Register a new suite in
      [test/e2e/tests/kgateway/suite_runner.go](/test/e2e/tests/kgateway/suite_runner.go).
    - Add it to one of the test kind clusters in
      [.github/workflows/e2e.yaml](/.github/workflows/e2e.yaml).
