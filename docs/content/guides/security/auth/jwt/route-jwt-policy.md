---
title: Route-level JWT policy
weight: 3
description: Learn how to set up JWT authentication and claim-based authorization for specific routes.
---

{{% notice note %}}
{{< readfile file="static/content/enterprise_only_feature_disclaimer" markdown="true">}} Route-level JWT policy is available in version 1.18.0 or later.
{{% /notice %}}

You can create a JWT policy that applies to specific routes with the VirtualService resource.

## About

Review how the VirtualService route option and VirtualHost option configurations interact with each other in the following table. 

The scenarios correspond with the steps in this guide. Each builds upon the previous scenario, updating the resources to test different scenarios. For example, you might update the stage of a JWT policy in a VirtualHost, or create a VirtualService with a JWT policy for a different issuer and JWKS provider.

| Scenario | Route option | VirtualHost option | Behavior |
| --- | --- | --- | --- |
| [1 - Protect all routes on the gateway](#gateway) | No JWT policy | JWT policy | All routes on the gateway require a JWT from the provider in the VirtualHost. |
| [2 - Protect a specific route](#route) | JWT policy | No JWT policy | Only the routes that are configured by the route option require a JWT. |
| [3 - Revise same-stage conflicts](#same-stage) | JWT policy at same stage | JWT policy at same stage | For routes configured by the route option, the route option configuration overrides the VirtualHost configuration. These routes require only the JWT that meets the route option JWT policy. JWTs that meet the VirtualHost do not work on these routes. Other routes on the gateway still require a JWT that meets the VirtualHost JWT policy. You cannot use a JWT with the provider from the route option for the other routes on the gateway. |
| [4 - Set up separate stages](#separate-stages) | JWT policy at different stage | JWT policy at different stage | The routes that are configured by the route option require two JWTs: one from the provider in the route option, and one from the provider in the VirtualHost. Make sure to configure the token source to come from different headers so that requests can pass in both tokens. Note that if you also configure RBAC policies on both options, the route option's RBAC policy takes precedence because only one RBAC policy is supported per route. |
| [5 - Add validation policy](#validation) | JWT policy at different stage | JWT policy at different stage with validation policy | Depending on the validation policy, the routes that are configured by the route option require at least one JWT. When the `validationPolicy` field is set to `ALLOW_MISSING` or `ALLOW_MISSING_OR_FAILED`, the JWT can be for the provider that is configured in either the route option or the VirtualHost. Note that if you set a permissive validation policy on both options, the route does not require any JWT authentication. Make sure to set up the validation policy according to your security requirements. |
| [6 - Delegated routes](#delegation) | Different JWT policies select delegated routes | N/A | For delegated routes, the JWT policies in the child route option override the parent route option when they differ. The same configuration interactions between the route-level options and the gateway-level VirtualHost options apply, as described in the previous scenarios. |

## Before you begin

1. [Install Gloo Gateway Enterprise Edition]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}).

2. Deploy the sample httpbin app.

   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     name: httpbin
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: httpbin
     labels:
       app: httpbin
   spec:
     ports:
     - name: http
       port: 8000
       targetPort: 80
     selector:
       app: httpbin
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: httpbin
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: httpbin
         version: v1
     template:
       metadata:
         labels:
           app: httpbin
           version: v1
       spec:
         serviceAccountName: httpbin
         containers:
         - image: docker.io/kennethreitz/httpbin
           imagePullPolicy: IfNotPresent
           name: httpbin
           ports:
           - containerPort: 80
   EOF
   ```

3. Create an Upstream for the httpbin destination.

   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: httpbin
   spec:
     kube:
       serviceName: httpbin
       serviceNamespace: default
       servicePort: 8000   
   EOF
   ```

## Scenario 1: All routes on gateway {#gateway}

Protect multiple routes that are configured on the gateway by configuring the options for a VirtualHost.

1. Create a VirtualService with a VirtualHost as in the following example.
   
   * The `domains` section sets the host domain that the VirtualHost listens on to `www.example.com`.
   * The `options` section creates a JWT policy with the details of the JSON Web Key Set (JWKS) server to use to verify the signature of JWTs in future protected requests.
   * The `routes` section of the VirtualHost defines two routes, `/get` and `/status/200`. 

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       options:
         jwt:
           providers:
             vhost-gateway-provider:
               issuer: solo.io
               jwks:
                 local:
                   key: |
                     -----BEGIN PUBLIC KEY-----
                     MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskFAGESgB22iOsGk/UgX
                     BXTmMtd8R0vphvZ4RkXySOIra/vsg1UKay6aESBoZzeLX3MbBp5laQenjaYJ3U8P
                     QLCcellbaiyUuE6+obPQVIa9GEJl37GQmZIMQj4y68KHZ4m2WbQVlZVIw/Uw52cw
                     eGtitLMztiTnsve0xtgdUzV0TaynaQrRW7REF+PtLWitnvp9evweOrzHhQiPLcdm
                     fxfxCbEJHa0LRyyYatCZETOeZgkOHlYSU0ziyMhHBqpDH1vzXrM573MQ5MtrKkWR
                     T4ZQKuEe0Acyd2GhRg9ZAxNqs/gbb8bukDPXv4JnFLtWZ/7EooKbUC/QBKhQYAsK
                     bQIDAQAB
                     -----END PUBLIC KEY-----
       routes:
       - matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
       - matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default 
   EOF
   ```

2. Send unauthenticated requests to both routes to verify that the JWT policy in the VirtualHost applies to all routes on the gateway.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get -H "host: www.example.com"
   curl -vik $(glooctl proxy url)/status/200 -H "host: www.example.com"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get -H "host: www.example.com"
   curl -vik localhost:31500/status/200 -H "host: www.example.com"
   ```
   {{% /tab %}}
   {{< /tabs >}}
   
   Example output: 
   
   ```
   HTTP/1.1 401 Unauthorized
   www-authenticate: Bearer realm="http://www.example.com/get"
   Jwt is missing
   ...
   HTTP/1.1 401 Unauthorized
   www-authenticate: Bearer realm="http://www.example.com/status/200"
   Jwt is missing
   ...
   ```

3. Create two environment variables to save the JWT tokens for the users Alice and Bob. To review the details of the token, you can use the [jwt.io](jwt.io) website. Optionally, you can create other JWT tokens by using the [JWT generator tool](https://github.com/solo-io/solo-cop/blob/main/tools/jwt-generator/README.md).

   {{< tabs >}}
   {{% tab name="Alice" %}}

   ```sh
   export ALICE_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyAiaXNzIjogInNvbG8uaW8iLCAib3JnIjogInNvbG8uaW8iLCAic3ViIjogImFsaWNlIiwgInRlYW0iOiAiZGV2IiwgImxsbXMiOiB7ICJvcGVuYWkiOiBbICJncHQtMy41LXR1cmJvIiBdIH0gfQ.I7whTti0aDKxlILc5uLK9oo6TljGS6JUrjPVd6z1PxzucUa_cnuKkY0qj_wrkzyVN5djy4t2ggE1uBO8Llpwi-Ygru9hM84-1m53aO07JYFya1VTDsI25tCRG8rYhShDdAP5L935SIARta2QtHhrVcd1Ae7yfTDZ8G1DXLtjR2QelszCd2R8PioCQmqJ8PeKg4sURhu05GlBCZoXES9-rtPVbe6j3YLBTodJAvLHhyy3LgV_QbN7IiZ5qEywdKHoEF4D4aCUf_LqPp4NoqHXnGT4jLzWJEtZXHQ4sgRy_5T93NOLzWLdIjgMjGO_F0aVLwBzU-phykOVfcBPaMvetg
   ```
   {{% /tab %}}
   {{% tab name="Bob" %}}
   ```sh
   export BOB_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyAiaXNzIjogInNvbG8uaW8iLCAib3JnIjogInNvbG8uaW8iLCAic3ViIjogImJvYiIsICJ0ZWFtIjogIm9wcyIsICJsbG1zIjogeyAibWlzdHJhbGFpIjogWyAibWlzdHJhbC1sYXJnZS1sYXRlc3QiIF0gfSB9.p7J2UFwnUJ6C7eXsFCSKb5b7ecWZ75JO4TUJHafjLv8jJ7GzKfJVk7ney19PYUrWrO4ntwnnK5_sY7yaLUBCJ3fv9pcoKyRtJTw1VMMTQsKkWFgvy-jEwc9M-D5lrUfR1HXGEUm6NBaj_Ja78XScPZb_-APPqMIvzDZU04vd6hna3UMc4DZE0wcnTjOqoND0GllHLupYTfgX0v9_AYJiKRAcJvol1W14dI7szpY5GFZtPqq0kl1g0sJPg-HQKwf7Cfvr_JLjkepNJ6A1lsrG8QbuUvMUAdaHzwLvF3L_G6VRjEte6okZpaq0g2urWpZgdNmPVN71Q_0WhyrJTr6SyQ
   ```
   {{% /tab %}}
   {{< /tabs >}}

4. Send another request to both routes. This time, include Alice's JWT token in the `Authorization` header. Because these JWT tokens were signed by the JWT issuer that is used in the VirtualHost, the request now succeeds. Verify that you get back a 200 HTTP response code.
   
   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get -H "host: www.example.com" --header "Authorization: Bearer $ALICE_TOKEN"
   curl -vik $(glooctl proxy url)/status/200 -H "host: www.example.com" --header "Authorization: Bearer $ALICE_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get -H "host: www.example.com" --header "Authorization: Bearer $ALICE_TOKEN"
   curl -vik localhost:31500/status/200 -H "host: www.example.com" --header "Authorization: Bearer $ALICE_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}
   
   Example output: 
   
   ```
   HTTP/1.1 200 OK
   ...
   HTTP/1.1 200 OK
   ...
   ```

[Back to table of scenarios](#about)

## Scenario 2: Specific routes {#route}

Use a route option in the VirtualService to protect a specific route with a JWT policy.

1. Update the VirtualService from the previous scenario as follows. 
   
   * Add an option to the `/get` route to configure a JWT policy.
   * Name each route so that you can configure a JWT policy. Unlike the VirtualHost option, the route option does not support unnamed routes.
   * Configure the JWT policy by using the `jwtProvidersStaged` option, instead of the `jwt` option. The `jwt` option does not support configuring a route-level JWT policy.
   * Set up a different provider than what is used in the VirtualHost.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       options:
         jwt:
           providers:
             vhost-gateway-provider:
               issuer: solo.io
               jwks:
                 local:
                   key: |
                     -----BEGIN PUBLIC KEY-----
                     MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskFAGESgB22iOsGk/UgX
                     BXTmMtd8R0vphvZ4RkXySOIra/vsg1UKay6aESBoZzeLX3MbBp5laQenjaYJ3U8P
                     QLCcellbaiyUuE6+obPQVIa9GEJl37GQmZIMQj4y68KHZ4m2WbQVlZVIw/Uw52cw
                     eGtitLMztiTnsve0xtgdUzV0TaynaQrRW7REF+PtLWitnvp9evweOrzHhQiPLcdm
                     fxfxCbEJHa0LRyyYatCZETOeZgkOHlYSU0ziyMhHBqpDH1vzXrM573MQ5MtrKkWR
                     T4ZQKuEe0Acyd2GhRg9ZAxNqs/gbb8bukDPXv4JnFLtWZ/7EooKbUC/QBKhQYAsK
                     bQIDAQAB
                     -----END PUBLIC KEY-----
       routes:
       - name: get
         matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
       - name: status-200
         matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default 
   EOF
   ```

2. Send a request to the `/status/200` route with a JWT for the provider in the VirtualHost, such as Bob. The request succeeds because the JWT policy from the VirtualHost still applies.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   
   ```
   < HTTP/1.1 200 OK
   ...
   ```

3. Repeat the previous request to the `/get` endpoint. Now, the request fails because you need a JWT from the provider in the route option.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 401 Unauthorized
   ...
   Jwt verification fails
   ```

4. Create an environment variable to save a JWT token for the user Carol. Carol's JWT comes from the provider in the route option. Optionally, you can review the token information by debugging the token in [jwt.io](jwt.io).

   ```shell
   export CAROL_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyAiaXNzIjogInNvbG8uaW8iLCAib3JnIjogInNvbG8uaW8iLCAic3ViIjogImNhcm9sIiwgInRlYW0iOiAiZmluYW5jZSIsICJsbG1zIjogeyAib3BlbmFpIjogWyAiZ3B0LTRvIiBdIH0gfQ.UdcOin9UrFdw_42eoypGsAi2eYE4Cr_oe0GYUPD6MePwr6TnWnny3cEQHFFRA9KdntjWBSPtZGKqNlOqg5Juf2-lt7NBLC3ly4esNEKrx_Ul5iKPxelKjNKzdOLdjITOa9FoZ9hZEn2lsn4MG-iftTXPeVn66-nWZryY0BE0Gt2fL1xvZe4Otbj598IY6Z5iPSxQ_fGNRe6f8boW31ePUgTiOthHs7OQv25-eiL8dl1BPBFYywFVGdiiSWrgd_hwRblMegJRhAiOZHRig1sK-NTKRKJpbLhukspM-CZaT1PJgjiOQb_1seeW7mvwUTlqDQA5FZKBCbhihb0TPfo6cw
   ```

5. Repeat the previous request to the `/get` endpoint, this time with Carol's token. Now, the request succeeds.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   ```

6. Still with Carol's token, send a request to the `/status/200` endpoint. This time, the request fails because Carol's token is not from the provider in the VirtualHost.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 401 Unauthorized
   ...
   Jwt verification fails
   ```

7. Update the VirtualService to remove the VirtualHost JWT option.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       routes:
       - name: get
         matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
       - name: status-200
         matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default 
   EOF
   ```

8. Repeat the request to the `/status/200` route with Carol's token. Now, the request succeeds because the gateway no longer enforces a JWT policy on all its routes.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/200 \
   -H "host: www.example.com" \
   --header "Authorization: Bearer $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   ```

[Back to table of scenarios](#about)

## Scenario 3: Same-stage conflicts {#same-stage}

As you saw in the previous scenario, route-level JWT policies configured in the VirtualService use the `jwtProvidersStaged` option to provide the JWT provider and server details. However, what happens if the gateway-level JWT policies in the VirtualHost are set at the same stage as the route option, such as before or after external auth? In this case, the route-level JWT policy takes precedence.

1. Update the VirtualService to demonstrate same-stage conflicts.
   
   * In the VirtualHost options, use the `jwtStaged` option to set the stage to `afterExtAuth`.
   * In the route option for the `/get` route, use the `jwtProvidersStaged` option to keep the `afterExtAuth` stage. 
   * In the route option, set the token source to the `x-after-ext-auth-bearer-token` header instead of the default `Authorization` header. This way, you can pass in different tokens on requests to the `/get` endpoint, such as Bob's token for the gateway-level JWT policy and Carol's token for the route-level JWT policy.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       options:
         jwtStaged:
           afterExtAuth:
             providers:
               vhost-gateway-provider:
                 issuer: solo.io
                 jwks:
                   local:
                     key: |
                       -----BEGIN PUBLIC KEY-----
                       MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskFAGESgB22iOsGk/UgX
                       BXTmMtd8R0vphvZ4RkXySOIra/vsg1UKay6aESBoZzeLX3MbBp5laQenjaYJ3U8P
                       QLCcellbaiyUuE6+obPQVIa9GEJl37GQmZIMQj4y68KHZ4m2WbQVlZVIw/Uw52cw
                       eGtitLMztiTnsve0xtgdUzV0TaynaQrRW7REF+PtLWitnvp9evweOrzHhQiPLcdm
                       fxfxCbEJHa0LRyyYatCZETOeZgkOHlYSU0ziyMhHBqpDH1vzXrM573MQ5MtrKkWR
                       T4ZQKuEe0Acyd2GhRg9ZAxNqs/gbb8bukDPXv4JnFLtWZ/7EooKbUC/QBKhQYAsK
                       bQIDAQAB
                       -----END PUBLIC KEY-----
       routes:
       - name: get
         matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   tokenSource:
                     headers:
                     - header: x-after-ext-auth-bearer-token
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
       - name: status-200
         matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default 
   EOF
   ```

2. Repeat the request to the `/get` endpoint with Carol's token. The request succeeds with only Carol's token, because the JWT policy on the route takes precedence and overwrites the VirtualHost configuration at the same stage for routes on the gateway.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   ```

3. Send a request to the `/status/200` endpoint without a valid token for the gateway-level JWT policy. Even though the VirtualHost configures a gateway-level JWT policy, the request succeeds, because the route option policy takes precedence.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   Jwt is missing
   ```

[Back to table of scenarios](#about)

## Scenario 4: Separate stages {#separate-stages}

Set up the VirtualService to configure JWT policies at different stages before and after external auth. This way, the protected routes require both tokens, such as Bob's token for the gateway-level JWT policy and Carol's token for the route-level JWT policy.

1. Update the VirtualService as follows.

   * **VirtualHost for gateway-level options**:
     * In the VirtualHost options, use the `jwtStaged` option to set the stage to `beforeExtAuth`. This way, the stage is different than the stage that is configured in the route option.
     * Add an RBAC policy to allow tokens from the `ops` team, which corresponds to Bob's token.

   * **Route options for the `/get` route**:
     * Use the `jwtProvidersStaged` option to keep the `afterExtAuth` stage. 
     * Keep the token source to the `x-after-ext-auth-bearer-token` header instead of the default `Authorization` header. This way, you can pass in both tokens on requests to the `/get` endpoint.
     * Add an RBAC policy that allows tokens from the `finance` team, which corresponds to Carol's token. In cases where multiple RBAC policies apply to a route, the VirtualService takes precedence.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       options:
         jwtStaged:
           beforeExtAuth:
             providers:
               vhost-gateway-provider:
                 issuer: solo.io
                 jwks:
                   local:
                     key: |
                       -----BEGIN PUBLIC KEY-----
                       MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskFAGESgB22iOsGk/UgX
                       BXTmMtd8R0vphvZ4RkXySOIra/vsg1UKay6aESBoZzeLX3MbBp5laQenjaYJ3U8P
                       QLCcellbaiyUuE6+obPQVIa9GEJl37GQmZIMQj4y68KHZ4m2WbQVlZVIw/Uw52cw
                       eGtitLMztiTnsve0xtgdUzV0TaynaQrRW7REF+PtLWitnvp9evweOrzHhQiPLcdm
                       fxfxCbEJHa0LRyyYatCZETOeZgkOHlYSU0ziyMhHBqpDH1vzXrM573MQ5MtrKkWR
                       T4ZQKuEe0Acyd2GhRg9ZAxNqs/gbb8bukDPXv4JnFLtWZ/7EooKbUC/QBKhQYAsK
                       bQIDAQAB
                       -----END PUBLIC KEY-----
         rbac:
          policies:
            viewer:
              nestedClaimDelimiter: .
              principals:
              - jwtPrincipal:
                  claims:
                    team: ops
       routes:
       - name: get
         matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   tokenSource:
                     headers:
                     - header: x-after-ext-auth-bearer-token
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
           rbac:
             policies:
               viewer:
                 nestedClaimDelimiter: .
                 principals:
                 - jwtPrincipal:
                     claims:
                       team: finance   
       - name: status-200
         matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
   EOF
   ```

2. Send a request to the `/get` endpoint with Carol's token. The request fails even though Carol's token is valid and meets the route-level RBAC policy. Instead, you need separate tokens for the two different stages of the JWT policies in the route-level and gateway-level options.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 401 Unauthorized
   ...
   Jwt is missing
   ```

3. Repeat the request to the `/get` endpoint with both Bob and Carol's tokens. Now, the request succeeds. Both tokens are required to pass the JWT policy at the different stages. The claims from Carol's token passes the RBAC policy.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN" \
   --header "Authorization: Bearer $BOB_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   ```

[Back to table of scenarios](#about)

## Scenario 5: Validation policy {#validation}

In the previous scenario, you protected a route by requiring JWT authentication at two stages of a request, before and after external auth. To do so, you configured separate JWT policies at the route and gateway layers with VirtualService and VirtualHost resources. But what if you want to enforce just one layer of JWT policy, without being picky about which JWT is used? 

You can achieve that by setting up a validation policy. The validation policy has several options as follows. For more details, see the [API reference docs](https://docs.solo.io/gloo-edge/main/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/jwt/jwt.proto.sk/#validationpolicy).

* `REQUIRE_VALID`: The default value, which allows only requests that have a valid JWT.
* `ALLOW_MISSING`: Let requests succeed when a JWT is missing. However, if an invalid JWT is provided, such as in an incorrect header or an expired token, the request fails.
* `ALLOW_MISSING_OR_FAILED`: Let requests succeed even when a JWT is missing or fails verification, such as an expired JWT.

{{% notice warning %}}
Note that if you set a permissive validation policy on both options, the route does not require any JWT authentication. Make sure to set up the validation policy according to your security requirements.
{{% /notice %}}

1. Verify that your current setup expects two JWT tokens by sending a request to the `/get` endpoint with Carol's token. The request fails even though Carol's token is valid and meets the route-level JWT policy. Instead, you need both tokens as required by the different JWT stages of the route and VirtualHost options.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 401 Unauthorized
   ...
   Jwt is missing
   ```

2. Update the VirtualService to add a validation policy to the VirtualHost that allows for a missing token. This way, you only have to include the JWT for the route-level configuration on the `/get` endpoint.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       options:
         jwtStaged:
           beforeExtAuth:
             validationPolicy: ALLOW_MISSING
             providers:
               vhost-gateway-provider:
                 issuer: solo.io
                 jwks:
                   local:
                     key: |
                       -----BEGIN PUBLIC KEY-----
                       MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskFAGESgB22iOsGk/UgX
                       BXTmMtd8R0vphvZ4RkXySOIra/vsg1UKay6aESBoZzeLX3MbBp5laQenjaYJ3U8P
                       QLCcellbaiyUuE6+obPQVIa9GEJl37GQmZIMQj4y68KHZ4m2WbQVlZVIw/Uw52cw
                       eGtitLMztiTnsve0xtgdUzV0TaynaQrRW7REF+PtLWitnvp9evweOrzHhQiPLcdm
                       fxfxCbEJHa0LRyyYatCZETOeZgkOHlYSU0ziyMhHBqpDH1vzXrM573MQ5MtrKkWR
                       T4ZQKuEe0Acyd2GhRg9ZAxNqs/gbb8bukDPXv4JnFLtWZ/7EooKbUC/QBKhQYAsK
                       bQIDAQAB
                       -----END PUBLIC KEY-----
         rbac:
          policies:
            viewer:
              nestedClaimDelimiter: .
              principals:
              - jwtPrincipal:
                  claims:
                    team: ops
       routes:
       - name: get
         matchers:
         - prefix: /get
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   tokenSource:
                     headers:
                     - header: x-after-ext-auth-bearer-token
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
           rbac:
             policies:
               viewer:
                 nestedClaimDelimiter: .
                 principals:
                 - jwtPrincipal:
                     claims:
                       team: finance   
       - name: status-200
         matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
   EOF
   ```

3. Repeat the request to the `/get` endpoint with Carol's token. Now, the request succeeds, even without Bob's token. The validation policy at the VirtualHost allows the JWT to be missing on requests.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/get \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 200 OK
   ...
   ```

[Back to table of scenarios](#about)

## Scenario 6: Delegated routes {#delegation}

In a [delegation scenario]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/delegation/" >}}), the JWT policy of child routes take precedence over parent routes. 

Note that this example focuses on setting up delegation with route-level configuration on the VirtualService and RouteTable resources. The example does not discuss gateway-level policies that are set by the VirtualHost. The same configuration interactions between the route-level and the gateway-level options as described in the previous scenarios still apply. For example, route-level JWT policies at the same stage as gateway-level JWT policies take precedence.

1. Update the VirtualService to delegate the `/status` route to a child RouteTable named `httpbin-child`. Leave the JWT policy configuration on the `/status` route to keep the original route-level JWT policy.

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: default
   spec:
     virtualHost:
       domains:
       - 'www.example.com'
       routes:
       - name: status
         matchers:
         - prefix: /status
         delegateAction:
           ref:
             name: httpbin-child
             namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 route-provider:
                   issuer: solo.io
                   tokenSource:
                     headers:
                     - header: x-after-ext-auth-bearer-token
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
   EOF
   ```

2. Create the child with two `/status` child routes of `/status/200` and `/status/418`.
   * The `/status/200` route does not have a JWT policy. As such, the parent JWT policy applies.
   * The `/status/418` route does have a JWT policy with a different JWT configuration than the parent route. This JWKS has a different `docs.xyz` issuer than the `solo.io` issuer of the parent route. This child JWT policy overwrites the parent policy.

   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: RouteTable
   metadata:
     name: httpbin-child
     namespace: default
   spec:
     routes:
       - name: status-200
         matchers:
          - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
       - name: status-418
         matchers:
          - prefix: /status/418
         routeAction:
           single:
             upstream:
               name: httpbin
               namespace: default
         options:
           jwtProvidersStaged:
             afterExtAuth:
               providers:
                 child-route-provider:
                   issuer: docs.xyz
                   tokenSource:
                     headers:
                     - header: x-after-ext-auth-bearer-token                  
                   jwks:
                     local:
                       key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/ZO8Qhfj6kB5BxndHds
                         x12rgJ2DyU0lvlbC4Ip1zTlULV/Fuy1uAqKbBRC9IyoFiYxuWTLbvpLv5SLnrIPy
                         f4nvX4oHGdyFrcwvCtKvcgtttB363HWiG0PZwSwEn0yMa7s4Rhmy9/ZSYm+sMZQw
                         8wKv40pYnBuqRv1DpfvZLOXvICCkd5f03zv1HQXIfO3YjXOy58vOkajpzTmx4q2A
                         UilrCJcR6tBMoAph5FiJxgRmdLziKx3QXukUSNWfrFVSL+D/BoQV+2TJDZjKfPgj
                         DDMKeb2OsonQ0me3VSw2gkdnE9cyIklXcne/+oKEqineG8a12JSfEibf29iLiIXO
                         gQIDAQAB
                         -----END PUBLIC KEY-----
   EOF
   ```

3. Send a request to the `/status/200` child route with Carol's token. The request succeeds, because Carol's token is from the `solo.io` issuer as required by the JWT policy of the parent route.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/200 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/200 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   HTTP/1.1 200 OK
   ...
   ```

4. Send another request to the `/status/418` child route with Carol's token. The request fails, because Carol's token is from the `solo.io` issuer as required by the JWT policy of the parent route, but the child route for `/status/418` requires its own JWT policy from a different issuer, `docs.xyz`.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/418 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/418 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $CAROL_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   < HTTP/1.1 401 Unauthorized
   Jwt issuer is not configured
   ...
   ```

5. Create an environment variable to save a JWT token for the user Dan. Dan's JWT comes from the `docs.xyz` provider in the `jwt-child` VirtualService. Optionally, you can review the token information by debugging the token in [jwt.io](jwt.io).

   ```shell
   export DAN_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJvcmciOiJkb2NzLnh5eiIsInN1YiI6ImRhbiIsInRlYW0iOiJvcHMiLCJpc3MiOiJkb2NzLnh5eiIsImxsbXMiOnsiY2xhdWRlIjpbIjMuNS1zb25uZXQiXX19.ny19crTIAsmlVlKjpdp52v4MJ037rNI5xyMoIqqA-jl6FK2XwhL0kn_xqvA3XDdKhMqy8hmH4nWbZPhHGzvs4gxXQW-_LPO0dDR5J_TOAqmR2j5epEyBWV7SvORGciG3nqpsJSBEzb6-artbbX8ehRpRZAyVvPQnfEYRkuPmmuzxUjyQpeWveCOJ9-HP3-PACqo2snMYoztsqR3mq2_kDWqvuxbhwuvFKEDQKe6tsvoVVc_7-qV4rHxiSmCQKagRtf0ALr7pzSOEVJ4JTWzRkkw5S5lO93sUbTittxEchZFEa7O3qKclvm5MqauF-UzFaB5YR9g2bUwGiRoYIV0BTA
   ```

6. Send another request to the `/status/200` child route, this time with Dan's token. The request fails, because Dans's token is not valid for the JWKS provider with the `solo.io` issuer as required by the JWT policy of the parent route.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/200 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $DAN_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/200 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $DAN_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   HTTP/1.1 401 Unauthorized
   Jwt issuer is not configured
   ...
   ```

7. Send another request to the `/status/418` child route, this time with Dan's token. The request succeeds, because the JWT is valid for the JWKS provider with the `docs.xyz` issuer of the JWT policy on the child route, which takes precedence over the parent route.

   {{< tabs >}}
   {{% tab name="LoadBalancer IP address or hostname" %}}
   ```sh
   curl -vik $(glooctl proxy url)/status/418 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $DAN_TOKEN"
   ```
   {{% /tab %}}
   {{% tab name="Local testing in Kind" %}}
   ```sh
   curl -vik localhost:31500/status/418 \
   -H "host: www.example.com" \
   --header "x-after-ext-auth-bearer-token: $DAN_TOKEN"
   ```
   {{% /tab %}}
   {{< /tabs >}}

   Example output: 
   ```
   HTTP/1.1 418 Unknown
   ...
    -=[ teapot ]=-

       _...._
     .'  _ _ `.
    | ."` ^ `". _,
    \_;`"---"`|//
      |       ;/
      \_     _/
        `"""`
   ```

[Back to table of scenarios](#about)

## Cleanup

{{< readfile file="static/content/cleanup.md" >}} 

1. Delete the httpbin sample app.

   ```sh
   kubectl delete -n default ServiceAccount httpbin
   kubectl delete -n default Service httpbin
   kubectl delete -n default Deployment httpbin
   kubectl delete -n default Upstream httpbin
   ```

2. Delete the resources that you created in this guide.

   ```sh
   kubectl delete -n default VirtualService httpbin
   kubectl delete -n default RouteTable httpbin-child
   ```