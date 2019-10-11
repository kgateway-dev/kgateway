---
title: Usage Statistics
weight: 100
description: Gloo's usage stats collection
---

## Usage Statistics

Gloo will periodically collect usage information from running instances. The details of this collection can be found [here](https://github.com/solo-io/go-checkpoint).
The checks are performed on each invocation of glootcl after a given interval. This interval can be changed by setting the env var `CHECKPOINT_TIMEOUT` in milliseconds. 
This information icludes the following: 

* Glooctl version
* Architecture
* Operating System

To turn off these checks simply set `CHECKPOINT_DISABLE=1`
