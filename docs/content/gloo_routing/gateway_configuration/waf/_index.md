---
title: Web Application Firewall (Enterprise)
weight: 30
---

## **What is a Web Application Firewall (WAF)**
A web application firewall (WAF) protects web applications by monitoring, filtering and blocking potentially harmful traffic and attacks that can overtake or exploit them. WAFs do this by intercepting and inspecting the network packets and uses a set of rules to determine access to the web application. In enterprise security infrastructure, WAFs can be deployed to an application or group of applications to provide a layer of protection between the applications and the end users.

Gloo now supports the popular Web Appplication Firewall framework/ruleset [ModSecurity](https://www.modsecurity.org/) 3.0.3.

## **WAF in Gloo**
Gloo Enterprise now includes the ability to enable the ModSecurity Web Application Firewall for any incoming and outgoing HTTP connections. The OWASP Core Rule Set is included by default and can be toggled on and off easily, as well as the ability to add or create custom rule sets. More information on the rule sets, and the rules language generally can be found [here](https://www.modsecurity.org/rules.html).

## **Why Mod Security**
API Gateways act as a control point for the outside world to access the various application services running in your environment. A Web Application Firewall offers a standard way to to inspect and handle all incoming traffic. Mod Security is one such firewall. ModSecurity uses a simple rules language to interpret and process incoming http traffic. There are many rule sets publically available, such as the [OWASP Core Rule Set](https://github.com/SpiderLabs/owasp-modsecurity-crs).

### Configuring WAF in Gloo
ModSecurity rule sets are defined in gloo in one of 3 places:

  * `HttpGateway`
  * `VirtualService`
  * `Route`

The precedence is as such: `Route` > `VirtualService` > `HttpGateway`. 

The configuration of the three of them is nearly identical at the moment, and follows the same pattern as other enterprise feaures in Gloo. The configuration is included in the extensions object of the various plugin sections, this process will be enumerated below, but first we will go over the general flow of configuring WAF in Gloo.

The WAF filter at it's core supports a list of `RuleSet` objects which are then loaded into the ModSecurity library. The Gloo API has a few conveniences built on top of that to allow easier access to the Core Rule Set. The  `RuleSet` Api looks as follows:
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

Each instance can be disabled, as well as include a list of `RuleSets`. These `RuleSets` are applied on top of each other in order. With the latter members overwriting the former. In addition, the `rule_str` is applied after the contents of the files in order to allow for fine-grained overrides.

A very simple example of a config is as follows:
```yaml
  ruleSets:
    - ruleStr: |
        # Turn rule engine on
        SecRuleEngine On
        # Deny requests which are container the header value user-agent:scammer
        SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
```
This tutorial will not do a deep dive on the rules as there is already plenty of information available, further documentation on the rules can be found [here](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual-(v2.x)). The purpose instead will be to understand how to apply the rules into new and existing Gloo configs.

As stated earlier, the above rule is very simple. It does only two things:

1. It enables the rules engine. This step is important, by default the rules engine is off, so it must be explicitally turned on. It can also be set to `DetectionOnly`, which runs the rules but does not perform any obtrusive actions.
2. It creates a rule which inspects the request header `"user-agent"`. If that specific header equals the value `"scammer"` then the request will be denied and return a `403` status.

This is a very basic example of the capabilities of the ModSecurity rules engine but useful in how it demonstrates it's implementation in Enterprise Gloo.

The following sections will explain how to enable this rule on the gateway level as well as on the virtual service level.

The following tutorials assume basic knowledge of Gloo and it's routing capabilities, as well a kubernetes cluster running Gloo Enterprise edition and the [petstore example]({{% versioned_link_path fromRoot="/gloo_routing/hello_world" %}}).

#### Http Gateway

The first option for configuring WAF is on the Http Gateway level on the Gateway. This can be useful if the goal is to apply the rules to all incoming requests to a given address, and not specific subsets.

Run the following command to edit the gateway object with the waf config:
```bash
kubectl edit gateway -n gloo-system gateway-proxy-v2
```

{{< highlight yaml "hl_lines=12-21" >}}
apiVersion: gateway.solo.io.v2/v2
kind: Gateway
metadata:
  name: gateway-proxy-v2
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames:
  - gateway-proxy-v2
  httpGateway:
    plugins:
      extensions:
        configs:
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
curl -v -H user-agent:scammer $(glooctl proxy url)/sample-route-1
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
{{< highlight yaml "hl_lines=6-16" >}}
...
spec:
  virtualHost:
    domains:
    - '*'
    virtualHostPlugins:
      extensions:
        configs:
          waf:
            settings:
              ruleSets:
              customInterventionMessage: 'ModSecurity intervention! Custom message details here..'
              - ruleStr: |
                  # Turn rule engine on
                  SecRuleEngine On
                  SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
...
{{< / highlight >}}

After this config has been successfully applied, run the curl command from above and the output should be the same.

The two methods outlined above represent the two main ways to apply basic rule string WAF configs to Gloo routes.

#### Core Rule Set

As mentioned earlier, the main free Mod Security rule set available is the owasp Core Rule Set. As with all other rule sets, the Core Rule Sets can be applied manually via the rule set configs, Gloo offers an easy way to apply the entire core rule set, and configure it.

In order to apply the core rule set add the following to the default virtual service.

{{< highlight yaml "hl_lines=7-34" >}}
spec:
  virtualHost:
    domains:
    - '*'
    name: gloo-system.default
    virtualHostPlugins:
      extensions:
        configs:
          waf:
            settings:
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
curl -v  $(glooctl proxy url)/sample-route-1
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
There are a couple important things to note from the config above. The `coreRuleSet` object is the first. By setting this object to non-nil the `coreRuleSet` is automatically applied to the gateway/vhost/route is has been added to. The Core Rule Set can be applied manually as well if a specific version of it is required which we do not mount into the container. The second thing to note is the config string. This config string is an important part of configuring the core rule set, an example of which can be found [here](https://github.com/SpiderLabs/owasp-modsecurity-crs/blob/v3.2/dev/crs-setup.conf.example).
