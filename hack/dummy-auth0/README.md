Based on agentgateway auth0 example: https://github.com/agentgateway/agentgateway/tree/485a299d2199cf7f792a8f7e55677f1a4898013b/examples/mcp-authentication

1. Build docker with the Maketarget 

```shell
VERSION=<version> make kind-build-and-load-dummy-auth0
```

2. Apply the mock IDP deployment 

```shell
kubectl apply -f test/e2e/features/agentgateway/mcp/testdata/auth0-mock-server.yaml
```

The mock server will always return the same JWT token and key pair, which has a 10year expiration date set. The values for 
the token returned are:
```shell
{
  "aud": "account",
  "exp": 1763676776,
  "iat": 1763673176,
  "iss": "https://kgateway.dev",
  "sub": "user@kgateway.dev"
}
```

The mock IDP provider will use a hard coded code `fixed_auth_code_123` for the `handle_authorize` endpoint. The mock
server also set the redirect url to `http:/localhost:8081/callback`, so when testing the MCP Inspector locally, 
make sure to expose this through the gateway and route configuration.  