# Installing on Nomad

Installation on Nomad requires the following:
- [Levant](https://github.com/jrasell/levant) installed on your local machine
- [Docker](https://github.com/jrasell/levant), [Consul](https://www.consul.io), [Vault](https://www.vaultproject.io), and [Nomad](https://www.nomadproject.io/) installed on the target host machine (which can also be your local machine)

> Note: This nomad job is experimental and designed to be used with a specific Vault + Consul + Nomad setup.

Inputs for the job can be tweaked by modifying `variables/variables-*.yaml` files

## Download the Installation Files

This tutorial uses files stored on the Gloo GitHub repository.

In order to install on Nomad, we'll want to clone the repository:

```
git clone --branch v0.18.23 https://github.com/solo-io/gloo
cd gloo/install/nomad
```

Installation files live in the `install/nomad` directory.

The Gloo [Nomad Job](https://www.nomadproject.io/docs/job-specification/index.html) lives at `install/nomad/jobs/gloo.nomad`
[Levant Variables](https://github.com/jrasell/levant) for the Gloo Nomad Job live at `install/nomad/variables/variables-{mac,linux}.yaml`

## Run the complete Demo

If your environment is set up with `docker`, `nomad`, `consul`, `vault`, and `levant`, you can simply run 
`demo.sh` to run a local demo of Gloo routing to the PetStore Nomad.

## Running Nomad, Consul and Vault

### Running Nomad Locally

If you've installed Nomad/Consul/Vault locally, you can use `launch-consul-vault-nomad-dev.sh` to run them on your local system.

If running locally (without Vagrant) on macOS, you will need to install the [Weave Net Docker Plugin](https://www.weave.works/docs/net/latest/install/plugin/plugin-v2/):

```bash
docker swarm init # if your docker host is not currently a swarm manager
docker plugin install weaveworks/net-plugin:latest_release --grant-all-permissions
docker plugin enable weaveworks/net-plugin:latest_release
docker network create --driver=weaveworks/net-plugin:latest_release --attachable weave

```

If running locally on Linux, you'll need to disable SELinux in order to run the demo (or add permission for docker containers to access `/` on their filesystem).

### Running Nomad Using Vagrant 

The provided `Vagrantfile` will run Nomad, Consul, and Vault inside a VM on your local machine. Download and install [HashiCorp Vagrant](https://www.vagrantup.com).

Then run the following command:

```bash
vagrant up
```

Ports will be forwarded to your local system, allowing you to access services on the following ports (on `localhost`):

|  service  | port | 
| ----- | ---- |  
| nomad | 4646 | 
| consul | 8500 | 
| vault | 8200 | 
| gloo/http | 8080 | 
| gloo/https | 8443 | 
| gloo/admin | 19000 | 

## Installing Gloo on Nomad (Linux)

```bash
levant deploy \
    -var-file variables/variables-linux.yaml \
    -address <nomad-host>:<nomad-port> \
    -consul-address <nomad-host>:<nomad-port> \
    jobs/gloo.nomad
```

If running locally or with `vagrant`, you can omit the `address` flags:

```bash
levant deploy \
    -var-file variables/variables-linux.yaml \
    jobs/gloo.nomad
```

## Installing Gloo on Nomad (Mac)

This option assumes you have Nomad, Consul, Vault, and Docker installed and running on your local macOS workstation.

```bash
levant deploy \
    -var-file variables/variables-mac.yaml \
    jobs/gloo.nomad
```

## Example

To run a test example, let's deploy the `petstore` application to Nomad as well:


### Deploy the PetStore on Nomad (Linux)

```bash
levant deploy \
    -var-file variables/variables-linux.yaml \
    -address <nomad-host>:<nomad-port> \
    -consul-address <nomad-host>:<nomad-port> \
    jobs/petstore.nomad
```

If running locally or with `vagrant`, you can omit the `address` flags:

```bash
levant deploy \
    -var-file variables/variables-linux.yaml \
    jobs/petstore.nomad
```

### Deploy the PetStore on Nomad (Mac)

This option assumes you have Nomad, Consul, Vault, and Docker installed and running on your local macOS workstation.

```bash
levant deploy \
    -var-file variables/variables-mac.yaml \
    jobs/petstore.nomad
```


### Create a Route to the PetStore

We can now use `glooctl` to create a route to the PetStore app we just deployed:

```bash
glooctl add route \
    --path-prefix / \
    --dest-name petstore \
    --prefix-rewrite /api/pets \
    --use-consul
```

```bash
{"level":"info","ts":"2019-08-22T17:15:24.117-0400","caller":"selectionutils/virtual_service.go:100","msg":"Created new default virtual service","virtualService":"virtual_host:<domains:\"*\" > status:<> metadata:<name:\"default\" namespace:\"gloo-system\" > "}
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| VIRTUAL SERVICE | DISPLAY NAME | DOMAINS | SSL  | STATUS  | LISTENERPLUGINS |             ROUTES             |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
| default         |              | *       | none | Pending |                 | / -> gloo-system.petstore      |
|                 |              |         |      |         |                 | (upstream)                     |
+-----------------+--------------+---------+------+---------+-----------------+--------------------------------+
```

> The `--use-consul` flag tells glooctl to write configuration to Consul Key-Value storage

And finally `curl` the Gateway Proxy:

```bash
curl <nomad-host>:8080/
```

If running locally or with Vagrant:

```bash
curl localhost:8080/
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Congratulations! You've successfully deployed Gloo to Nomad and created your first route.

Most of the existing tutorials for Gloo can be reused with Nomad, however glooctl commands should be 
used with the `--use-consul` flag.
