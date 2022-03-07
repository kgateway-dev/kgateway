<!--
---
title: Debugging
weight: 80
description: TODO
---


It would be great to have a set of steps for how to debug issues. I’m thinking:
- Enabling debug logging for Envoy - check other debug docs
- Example of a typical log output for a single GraphQL API request - run through getting started (also need to verify small things like changed svc names or exact output in the GS)
- Checking status on relevant CRs (GraphQLSchema, VirtualService, Gateway) - glooctl check will work for the last two but not the former
- Checking metrics to see activity - link to metrics page
-->