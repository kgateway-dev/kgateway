NATS Streaming Demo
====================

This document outlines a demo of gloo composing ('glooing') enabling scale for an application using NATS Streaming.

# Prerequisites
In this demo, we will use the following command line tools:
- `docker-compose` to create the environment.
- `glooctl` to interact with gloo.

Additionally, some of the demo commands reference files, so first `cd` to the folder that contains 
this readme (usually this is `cd $GOPATH/src/github.com/solo-io/gloo/example/nats`).

# Setup the environment

Bring up all the containers:
```
$ docker-compose up --build
```

Initialize gloo upstreams:
```
$ glooctl --gloo-config-dir gloo-config/_gloo_config/ --secret-dir gloo-config/_gloo_secrets/ upstream create  -f nats-upstream.yaml
$ glooctl --gloo-config-dir gloo-config/_gloo_config/ --secret-dir gloo-config/_gloo_secrets/ upstream create  -f website-upstream.yaml
$ glooctl --gloo-config-dir gloo-config/_gloo_config/ --secret-dir gloo-config/_gloo_secrets/ upstream create  -f analytics.yaml
```

Initialize gloo virtualservice:
```
$ glooctl --gloo-config-dir gloo-config/_gloo_config/  virtualservice create  -f virtualservice.1.5.yaml
```

You can test that everything is in order using:
```
$ glooctl --gloo-config-dir gloo-config/_gloo_config/  upstream get -o yaml
$ glooctl --gloo-config-dir gloo-config/_gloo_config/  virtualservice get -o yaml
```

Once everything is up and running, you can the demo website in your browser, and continue there: http://localhost:8080

You can visit http://localhost:3000 and login with username *admin* and password *admin* to monitor Gloo.

# Cleanup:

To stop the containers, hit CNTRL+C on in the terminal where `docker-compose up` was invoked.

Delete containers:
```
docker-compose down
```

Remove gloo config:

```bash
$ rm gloo-config/_gloo_config/upstreams/*
$ rm gloo-config/_gloo_config/virtualservices/*
```
