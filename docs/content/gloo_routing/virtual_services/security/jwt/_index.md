---
title: Json Web Tokens (JWT)
weight: 30
description: Introduction to JWT and what they are used for
---

## What are Json Web Tokens
Json Web Tokens, or JWT for short, are a standard way to carry verifiable identity information.
This can be used for authentication. The advantage of using JWTs is that since they are a standard
format and cryptographically signed, they can usually be verified without contacting an external
authentication server. To support this use case, the application server verifying the JWTs needs to
be setup with a private key for verification - JWTs signed by that key will be verified by the
application server. Those who are not will be rejected (usually via an HTTP 401 response code).

JWTs are useful in various scenarios, such as:

- OpenID Connect's `id_token` is a JWT. The `id_token` is used to identify the End User (*Resource Owner* 
  in OIDC/OAuth terminology) and is usually sent by the client (phone app or web-browser) to
  the cloud back-end (*Resource Server* in OIDC/OAuth terminology)
- Kubernetes uses JWT as service accounts secrets within Pods. A program running in a Pod can
  use this JWT to authenticate with the Kuberenetes API server with the permissions of the 
  service account.

## How is a JWT structured

A JWT has three parts:

- The header
- The payload
- The signature

All three parts are combined with the "." character to form the final token. The header has some
metadata on the JWT (like the signing algorithm). The payload carries `claims` that the token makes 
(more on that in the next section). And finally the signature part is a cryptographic signature that 
signs the header and the payload.

## How does a JWT Carry Identity Information

Inside the JWT various *claims* are encoded; claims provide identity information. A few standard claims are:

- `iss` - The entity that issued the token
- `sub` - Subject of the token. This is usually a user id.
- `aud` - The audience the token was issued for. This is an important security feature that makes sure
          that a token issued for one use cannot be used for other purposes.

The claims are encoded as a JSON object, and then encoded with base64 to form the payload of the JWT

## How is a JWT Verified

Most commonly asymmetric encryption is used to sign JWTs. To verify them a public key is used. This 
has the advantage of making verification easy - the public key can be distributed as it is not secret
and cannot be used to sign new JWTs. The JWT can be independently verified by anyone using the public key.

## JWTs in Gloo
Gloo supports JWT verification using the JWT extension. You can define multiple JWT providers.
In each provider you can specify where to find the keys required for JWT verification, the 
values for the issuer and audience claims to verify, as well as [other settings](../../../../api/github.com/solo-io/solo-projects/projects/gloo/api/v1/plugins/jwt/jwt.proto.sk/#provider).

We have a few guides that go into more detail:

- [JWT and Access Control](./access_control) - Demonstrates how to use Gloo as an internal API Gateway
  in a Kubernetes environment. Gloo is used to verify Kuberentes service account JWTs and to define
  an RBAC policy on what those service accounts are allowed to access.
- [JWT Claim Based Routing](./claim_routing) - Shows a method of using JWT claims to perform routing
  decisions. This can be used, for example, to send your own organization employees to a canary build
  of your app while sending other traffic to the primary/production build of the app.
