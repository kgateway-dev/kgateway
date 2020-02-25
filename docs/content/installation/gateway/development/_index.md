---
title: "Installing Gloo Gateway on Your Local System"
menuTitle: "Local System"
description: How to install Gloo to run in Gateway Mode on your local system.
weight: 50
---

Gloo supports running in Gateway Mode on your local system by using Docker and Docker Compose. This is ideal if you are trying to experiment with Gloo without running a server or virtual machine. There are two possible scenarios:

1. [Consul & Vault]({{% versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul/" %}}) - Deploy Gloo with Docker Compose and use a dev instance of HashiCorp Consul and Vault
1. [Local Files]({{% versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file/" %}}) - Deploy Gloo with Docker Compose and use the local filesystem for config data and secrets