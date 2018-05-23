# Installing on Docker

## Prerequisite

 1. [Docker](https://www.docker.com/)
 2. [glooctl](https://github.com/solo-io/glooctl) v0.2.6 or above.

## Install

 Run the following command to install the files necessary for docker-compose to a folder:

 ```
 glooctl install docker [folder]
 ```

The folder is the folder you want to install the latest version of gloo related files. This sets up the file based storage for gloo. This will also update the glooctl configuration.

For example,

```
glooctl install docker gloo-tutorial
Gloo setup successfully.
Please switch to directory '/Users/ashish/Projects/gloo-tutorial', and run "docker-compose up"
to start gloo.
```

## Running Gloo

You can run `gloo` with docker-compose by changing to the folder and running:

```
cd [folder]

docker-compose up
```

You can check gloo services are running by running `docker ps`

For example,

```
docker ps

CONTAINER ID        IMAGE                             COMMAND                  CREATED             STATUS              PORTS                                                                      NAMES
6aea10e3ac4e        soloio/function-discovery:0.2.5   "/function-discovery…"   49 seconds ago      Up 46 seconds                                                                                  gloo-tutorial_function-discovery_1
d42a9dc94275        soloio/envoy:v0.1.6-132           "envoy -c /config/en…"   49 seconds ago      Up 47 seconds       0.0.0.0:8080->8080/tcp, 0.0.0.0:8443->8443/tcp, 0.0.0.0:19000->19000/tcp   gloo-tutorial_ingress_1
1a37c031adf3        soloio/control-plane:0.2.5        "/control-plane --st…"   49 seconds ago      Up 46 seconds       0.0.0.0:8081->8081/tcp                                                     gloo-tutorial_control-plane_1
```


Everything should be up and running. If this process does not work, please [open an issue](https://github.com/solo-io/gloo/issues/new). We are happy to answer questions on our diligently staffed [Slack channel](https://slack.solo.io)

See [Getting Started on Docker](../getting_started/docker/1.md) to get started creating routes with Gloo.