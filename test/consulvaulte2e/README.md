# Consul/Vault Tests

## Setup
The consul vault test downloads and runs vault and is disabled by default. To enable, set `RUN_VAULT_TESTS=1` and `RUN_CONSUL_TESTS=1` in your local environment.

## Note to developers:
These tests set up and run Gloo with a different than normal path for generating the runtime options.

If you have made changes to the setup loop and these tests are suddenly failing, you may need to make corresponding 
changes here:
https://github.com/solo-io/gloo/blob/61d35b0d4ce3b2b28ed47c7be06d9acaadf37074/test/services/gateway.go#L249

## TODO: Instructions for running locally 

