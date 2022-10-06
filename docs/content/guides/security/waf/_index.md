---
title: Web Application Firewall
weight: 60
description: Filter, monitor, and block potentially harmful HTTP traffic.
---

{{% notice note %}}
The WAF feature was introduced with **Gloo Edge Enterprise**, release 0.18.23. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

## **What is a Web Application Firewall (WAF)**
A web application firewall (WAF) protects web applications by monitoring, filtering and blocking potentially harmful 
traffic and attacks that can overtake or exploit them. WAFs do this by intercepting and inspecting the network packets 
and uses a set of rules to determine access to the web application. In enterprise security infrastructure, WAFs can be 
deployed to an application or group of applications to provide a layer of protection between the applications and the 
end users.

Gloo Edge supports the popular Web Application Firewall framework/ruleset [ModSecurity](https://www.github.com/SpiderLabs/ModSecurity) 3.0.4.

## **WAF in Gloo Edge**
Gloo Edge Enterprise includes the ability to enable the ModSecurity Web Application Firewall for any incoming and outgoing HTTP connections. There is support for configuring rule sets based on the OWASP Core Rule Set as well as custom rule sets. More information on available rule sets, and the rules language generally, can be found [here](https://www.modsecurity.org/rules.html).

## **Why ModSecurity**
API Gateways act as a control point for the outside world to access the various application services running in your environment. A Web Application Firewall offers a standard way to inspect and handle all incoming traffic. ModSecurity is one such firewall. ModSecurity uses a simple rules language to interpret and process incoming http traffic. There are many rule sets publicly available, such as the [OWASP Core Rule Set](https://github.com/coreruleset/coreruleset).

### Configuring WAF in Gloo Edge
ModSecurity rule sets are defined in gloo in one of 3 places:

  * `HttpGateway`
  * `VirtualService`
  * `Route`

The precedence is as such: `Route` > `VirtualService` > `HttpGateway`. 

The configuration of the three of them is nearly identical and follows the same pattern as other enterprise features in Gloo Edge. 
The configuration is included in the `options` object of the `httpGateway`. This process will be outlined 
below, but first we will go over the general flow of configuring WAF in Gloo Edge.

The WAF filter supports a list of `RuleSet` objects which are loaded into the ModSecurity library. 
The Gloo Edge API has a few conveniences built on top of that to allow easier access to the OWASP Core Rule Set (via the [coreRuleSet](#core-rule-set) field). 
The  `RuleSet` Api looks as follows:

```proto
message ModSecurity {
    // Disable all rules on the current route
    bool disabled = 1;
    // Global rule sets for the current http connection manager
    repeated RuleSet rule_sets = 2;
    // Custom message to display when an intervention occurs
    string custom_intervention_message = 3;
}

message RuleSet {
    // string of rules which are added directly
    string rule_str = 1;
    // array of files to include
    repeated string files = 3;
}
```

Each instance can be disabled independently. Each individual instance can include a list of `RuleSet`s, applied on top of each other in order with the latter members overwriting the former. In addition, the `rule_str` is applied after the contents of the files to allow for fine-grained overrides.

A very simple example of a config is as follows:
```yaml
  ruleSets:
    - ruleStr: |
        # Turn rule engine on
        SecRuleEngine On
        # Deny requests which are container the header value user-agent:scammer
        SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
```
This tutorial will not do a deep dive on the rules; further documentation on the these can be found [here](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-(v2.x)). The purpose instead will be to understand how to apply the rules into new and existing Gloo Edge configs.

The above rule is very simple. It does only two things:

1. Enables the rules engine. This step is important; by default the rules engine is off, so it must be explicitly turned on. It can also be set to `DetectionOnly` which runs the rules but does not perform any obtrusive actions.
2. Creates a rule which inspects the request header `"user-agent"`. If the value of that header equals `"scammer"`, then the request will be denied and a `403` status will be returned.

This very basic example of the capabilities of the ModSecurity rules engine demonstrates its implementation in Enterprise Gloo Edge.

The following sections will explain how to enable this rule on the gateway level as well as on the virtual service level.

The following tutorials assume basic knowledge of Gloo Edge and its routing capabilities. A Kubernetes cluster running Gloo Edge Enterprise edition and the [petstore example]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) is also required.

#### Http Gateway

The first option for configuring WAF is on the Http Gateway level on the Gateway. This can be useful if the goal is to apply the rules to all incoming requests to a given address, and not specific subsets.

Run the following command to edit the gateway object with the waf config:
```bash
kubectl edit gateway -n gloo-system gateway-proxy
```

{{< highlight yaml "hl_lines=11-19" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames:
  - gateway-proxy
  httpGateway:
    options:
      waf:
        customInterventionMessage: 'ModSecurity intervention! Custom message details here..'
        ruleSets:
        - ruleStr: |
            # Turn rule engine on
            SecRuleEngine On
            SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
  useProxyProto: false
{{< / highlight >}}

Once this config has been accepted run the following to test that the rule has been applied
```bash
curl -v -H User-Agent:scammer $(glooctl proxy url)/sample-route-1
```
should respond with
```
*   Trying 192.168.99.144...
* TCP_NODELAY set
* Connected to 192.168.99.144 (192.168.99.144) port 32683 (#0)
> GET /sample-route-1 HTTP/1.1
> Host: 192.168.99.144:32683
> Accept: */*
> user-agent:scammer
>
< HTTP/1.1 403 Forbidden
< content-length: 55
< content-type: text/plain
< date: Tue, 29 Oct 2019 19:53:38 GMT
< server: envoy
<
* Connection #0 to host 192.168.99.144 left intact
ModSecurity intervention! Custom message details here..
```

As can be seen above from the curl output, the request was rejected by the waf filter, and the status 403 was returned.

#### Virtual Service

Firstly, remove the extension config from the gateway which was added in the section above. Once the config has been removed from the gateway, add it to the default virtual service like so:
```bash
kubectl edit virtualservices.gateway.solo.io -n gloo-system default
```
{{< highlight yaml "hl_lines=6-13" >}}
...
spec:
  virtualHost:
    domains:
    - '*'
    options:
      waf:
        customInterventionMessage: 'ModSecurity intervention! Custom message details here..'
        ruleSets:
        - ruleStr: |
            # Turn rule engine on
            SecRuleEngine On
            SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
...
{{< / highlight >}}

After this config has been successfully applied, run the curl command from above and the output should be the same.

The two methods outlined above represent the two main ways to apply basic rule string WAF configs to Gloo Edge routes.

#### Dynamically Load Rule Set 

In some cases you may want your rulesets to update automatically when you make changes

If you are using ruleSets as shown above with a ruleString passed in WAF settings then you will already have this behavior. If you would prefer to place your rules outside of the CRD definition you can do so with kubernetes configmaps 

Create a file containing the same ruleStr from the first example 

```bash
cat <<EOF > wafruleset.conf
SecRuleEngine On
SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
EOF
```

Now create a configmap from this file 
Note: Kubernetes Configmaps can be created from multiple files. The rulesets from the files will be ordered based on the filenames, not the order they appear in the configmaps Data map. 

```bash
kubectl --namespace=gloo-system create configmap wafruleset --from-file=wafruleset.conf
```

And view this configmap

```bash
kubectl --namespace=gloo-system get configmap wafruleset -oyaml
```

We should see 

```yaml
apiVersion: v1
kind: ConfigMap
data:
  wafruleset.conf: |
    SecRuleEngine On
    SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
```

In order to use the configmap we now need to update our WAF settings using the CustomConfigMapRuleSets field 

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames:
  - gateway-proxy
  httpGateway:
    options:
      waf:
        customInterventionMessage: 'ModSecurity intervention! Custom message details here..'
        customConfigMapRuleSets:
          - name: wafruleset
            namespace: gloo-system
  useProxyProto: false
```

You can now test this in the same way as before with a curl command - the behavior will be the same

```bash
curl -v -H User-Agent:scammer $(glooctl proxy url)/sample-route-1
```

#### Core Rule Set

{{% notice warning %}}
Using the `rbl` modsecurity rule in Gloo Edge will cause envoy performance issues and should be avoided. If `rbl` blacklisting is a requirement, an [extauth plugin]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth">}}) can be used to query the rbl list and forbid spam IPs.
{{% /notice %}}

As mentioned earlier, the main free ModSecurity rule set available is the OWASP Core Rule Set. As with all other rule sets, the Core Rule Set can be applied manually via the rule set configs, Gloo Edge offers an easy way to apply the entire Core Rule Set, and configure it.

In order to apply the Core Rule Set add the following to the default virtual service. Without the coreRuleSet field, the OWASP Core Rule Set files will not be included.

{{< highlight yaml "hl_lines=7-33" >}}
spec:
  virtualHost:
    domains:
    - '*'
    name: gloo-system.default
    options:
      waf:
        coreRuleSet:
          customSettingsString: |
              # default rules section
              SecRuleEngine On
              SecRequestBodyAccess On
              # CRS section
              # Will block by default
              SecDefaultAction "phase:1,log,auditlog,deny,status:403"
              SecDefaultAction "phase:2,log,auditlog,deny,status:403"
              # only allow http2 connections
              SecAction \
                "id:900230,\
                  phase:1,\
                  nolog,\
                  pass,\
                  t:none,\
                  setvar:'tx.allowed_http_versions=HTTP/2 HTTP/2.0'"
              SecAction \
              "id:900990,\
                phase:1,\
                nolog,\
                pass,\
                t:none,\
                    setvar:tx.crs_setup_version=310"
{{< / highlight >}}

Once this config has been accepted run the following to test that it works.
```bash
curl -v $(glooctl proxy url)/sample-route-1
```
should respond with
```
*   Trying 192.168.99.145...
* TCP_NODELAY set
* Connected to 192.168.99.145 (192.168.99.145) port 30093 (#0)
> GET /sample-route-1 HTTP/1.1
> Host: 192.168.99.145:30093
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 403 Forbidden
< content-length: 33
< content-type: text/plain
< date: Wed, 30 Oct 2019 13:10:38 GMT
< server: envoy
<
* Connection #0 to host 192.168.99.145 left intact
ModSecurity: intervention occurred
```
There are a couple important things to note from the config above. The `coreRuleSet` object is the first. By setting this object to non-nil the `coreRuleSet` is automatically applied to the gateway/vhost/route is has been added to. The Core Rule Set can be applied manually as well if a specific version of it is required which we do not mount into the container. The second thing to note is the config string. This config string is an important part of configuring the Core Rule Set, an example of which can be found [here](https://github.com/SpiderLabs/owasp-modsecurity-crs/blob/v3.2/dev/crs-setup.conf.example).


## IP Allowlist

A very common use case in many organizations is restricting access for an API to a specific IP address or subnet range. This requirement manifests in many ways, such as maintaining an access control list (ACL) for certain internal services or enforcing network boundaries between various, discrete environments.

We can utilize the WAF filter and custom modsecurity rules to easily satisfy these requirements. To illustrate this concept, we will outline how to restrict access to a service to our workstation's IP along with any other hosts that are in the same `/16` CIDR block as our IP. This IP whitelisting will be based on the original source IP of a request originating from a developer workstation and flowing through a cloud `LoadBalancer` provisioned by the Kubernetes `Service`.

Since we will be whitelisting IPs that travel through our cloud provider's `LoadBalancer`, we need to ensure the original source IP is preserved. Most commonly, this is configured on `Service` resources by setting the `externalTrafficPolicy: Local`. For our purposes, we will patch the 'gateway-proxy' `Service`:

```
spec:
  externalTrafficPolicy: Local
```

Now the workload behind this `Service` will correctly see the original client IP as the remote address connecting to it. We can then utilize this address in our WAF rules to implement IP whitelisting. In this case, we will add the following patch to our `VirtualService`:

```
spec:
  virtualHost:
    options:
      waf:
        ruleSets:
        - ruleStr: |
            SecRuleEngine On
            SecRule REMOTE_ADDR "!@ipMatch 173.175.0.0/16" "phase:1,deny,status:403,id:1,msg:'block ip'"
```

We are applying a WAF rule at the `virtualHost` level, meaning that the rule will be applied to all routes for this `VirtualService`. The rule we are applying will cause modsecurity to inspect the remote address for the request being processed and if the IP address does not fall in the `173.175.0.0/16` network range, the request will be denied with a 403 status code.

## gRPC 

If you have configured WAF, gRPC calls to a VirtualService fronting a gRPC services may timeout if the gRPC call is a stream
as the WAF filter requires an end stream to be specified to finish evaluating intervention. To avoid this timeout, add `requestHeadersOnly: true` and `responseHeadersOnly: true` to the WAF config:

```yaml
waf:
  requestHeadersOnly: true
  responseHeadersOnly: true
  customInterventionMessage: ModSecurity intervention! Custom message details here..
  ruleSets:
  - ruleStr: |
      # Turn rule engine on
      SecRuleEngine On
      SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
```

## Audit Logging

Audit Logging is supported starting Gloo Edge Enterprise v1.4.0, but it works differently than in other ModSecurity integrations.
ModSecurity native audit logging is not a good fit for Envoy/Kubernetes cloud native environments.
ModSecurity has 3 logging engines. They are not a good fit for the following reasons:
1. Serial - all logs written to one file, which globally locks on each write. This will be horrendous
   for performance, as all envoy worker threads will be blocked when trying to log while one-by-one they write to the file.
2. Parallel - each log entry is written to its unique file. This will impact performance as this file IO is 
   outside the envoy worker thread event loop, thus blocking it and increasing latency. In addition,
   now we will have many logs files to collect from the pod. This means we'll a need writeable volume attached via sidecar 
   container to collect them which increases complexity.
3. Http - perform an http callout to an external logging server - like the other methods, this http call
   is done outside the envoy event loop, blocking it until it is completed, which will increase latency.

With this integration, we take a more cloud native approach. We expose the audit logs as part
of envoy's access logging. This means that directives that configure the audit engine itself 
([SecAuditEngine](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditEngine), [SecAuditLog](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLog), [SecAuditLogStorageDir](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLogStorageDir), [SecAuditLogType](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLogType), ...) are
**ignored even if they are set**.
This is **intentional** - to make sure that ModSecurity doesn't degrade
envoy performance. While the way we emit the logs is different, you have _all the features_ that 
ModSecurity audit-logging provides:
- You can use the `action` property of the [audit logging configuration]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/waf/waf.proto.sk/#auditlogging" %}}) instead of [SecAuditEngine](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditEngine) to choose when to log.
- You can still use the [SecAuditLogParts](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLogParts), 
[SecAuditLogRelevantStatus](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLogRelevantStatus) and (assuming action is RELEVANT_ONLY) `noauditlog` features of ModSecurity.
- The format of the log is controlled by [SecAuditLogFormat](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-%28v2.x%29#SecAuditLogFormat).

As envoy access logs have their own filtering mechanism built in, we provide two methods of exposing the audit logs via the access logs. Each method has different CPU/Memory trade-offs.

- DynamicMetadata - This processes the audit log eagerly every time it is required. This may increase CPU
as the audit log will be computed even if envoy doesn't end-up logging it. To use, place `%DYNAMIC_METADATA("io.solo.filters.http.modsecurity:audit_log")%` in the access log.
- FilterState - this will only generate an AuditLog lazily - only if the envoy access log is about to log it. This will use
less CPU if the message is not logged, but may use more memory, as the ModSecurity transaction 
will linger in memory longer. To use, place `%FILTER_STATE(io.solo.modsecurity.audit_log)%` in the access log.

{{% notice warning %}}
[Data Loss Prevention (DLP)]({{% versioned_link_path fromRoot="/guides/security/data_loss_prevention/" %}}) is **not supported** with FilterState access logging.
{{% /notice %}}

We recommend testing both in an environment similar to your prod environment, to understand which approach
is better for your specific use-case.

Let's see this in action!

To enable audit logging, edit the [auditLogging]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/waf/waf.proto.sk/#auditlogging" %}}) field in your 
[WAF settings]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/waf/waf.proto.sk/#settings" %}}).

For example, lets edit our `VirtualService` with some
rules and audit logging:

{{< highlight yaml "hl_lines=6-8 15" >}}
...
spec:
  virtualHost:
    options:
      waf:
        auditLogging:
          action: ALWAYS
          location: FILTER_STATE
        ruleSets:
        - ruleStr: |
            # Turn rule engine on
            SecRuleEngine On
            # Set audit log format to JSON. leave this out to use the
            # regular string format.
            SecAuditLogFormat JSON
            # A simple rule to catch a "scammer"
            SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
{{< / highlight >}}

Enable access logs:

{{< highlight yaml "hl_lines=13-19" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stderr
          stringFormat: "%FILTER_STATE(io.solo.modsecurity.audit_log)%\n"
{{< / highlight >}}

Generate a request that will trigger ModSecurity:
```shell
curl -v $(glooctl proxy url) -H "user-agent: scammer"
```

Check the logs:
```shell
kubectl -n gloo-system logs deploy/gateway-proxy
```

and you should see the following output:
```json
{"transaction":{"request":{"http_version":1.1,"body":"","headers":{":path":"/api/pets/1","x-forwarded-proto":"http","accept":"*/*","host":"172.17.0.2:32608","user-agent":"scammer",":authority":"172.17.0.2:32608",":method":"GET","x-request-id":"a91986b2-5928-427e-a557-15f5cbeec104"},"uri":"/api/pets/1","method":"GET"},"host_port":0,"host_ip":"","unique_id":"158879653826.189180","client_ip":"10.244.0.1","time_stamp":"Wed May  6 20:22:18 2020","messages":[{"message":"blocked scammer","details":{"maturity":"0","match":"Matched \"Operator `Rx' with parameter `scammer' against variable `REQUEST_HEADERS:user-agent' (Value: `scammer' )","reference":"o0,7v133,7","lineNumber":"7","ruleId":"107","severity":"0","file":"\u003c\u003creference missing or not informed\u003e\u003e","ver":"","rev":"","data":"","tags":[],"accuracy":"0"}}],"client_port":48839,"producer":{"secrules_engine":"Enabled","modsecurity":"ModSecurity v3.0.4 (Linux)","components":[],"connector":"envoy v0.1.0"},"response":{"http_code":200,"headers":{}},"server_id":"4ce9d7cf1298296878f1ae2e9d40de00b290a3a4"}}
```

{{% notice tip %}}
By default Envoy flushes log files every 10 seconds. To see logs faster while testing this guide, we can set it
to a lower value. To do so, edit the gateway proxy deployment
and add the `--file-flush-interval-msec 100` to the envoy arguments.

The arguments should look like so:
```
      - args:
        - --disable-hot-restart
        - --file-flush-interval-msec
        - "100"
```
{{% /notice %}}

### Audit Log Formatting

Alternatively, specify a `jsonFormat` format string to dump ModSecurity logs as part of a larger JSON object:

{{< highlight yaml "hl_lines=13-21" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stderr
          jsonFormat:
            method: '%REQ(:METHOD)%'
            response_code: '%RESPONSE_CODE%'
            waf: "%FILTER_STATE(io.solo.modsecurity.audit_log)%"
{{< / highlight >}}

The above configuration will log a JSON object containing the request method, response code, and any logs obtained from ModSecurity when a request is processed.