Note: This nomad job is experimental and designed to be used with a
specific vault+consul+nomad setup.

See `launch-consul-vault-nomad-dev.sh` to see how we run Nomad in a way
that supports `install.nomad`

See `get-gateway-url.sh` to see how we get the URL of the Gateway container
that's running inside of docker (through nomad).

To install:

`nomad run install.nomad`