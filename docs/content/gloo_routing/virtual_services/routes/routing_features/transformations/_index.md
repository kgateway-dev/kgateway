---
title: Transformations
weight: 10
description: Transform requests and responses by using the Gloo transformation filter
---

[Inja Templates](https://github.com/pantor/inja/tree/74ad4281edd4ceca658888602af74bf2050107f0) give you a powerful way
to process JSON formatted data. For example, if you had a message body that contained the JSON `{ "name": "world" }`
then the Inja template `Hello {{ name }}` would become `Hello world`. The template variables, e.g., `{{ name }}`, is
used as the key into a JSON object and is replaced with the key's associated value.

{{% notice note %}}
Inja Templates default to using `.` notation for JSON keys, i.e., `address.street` => `{ "address": { "street": "value" } }`.
If `advancedTemplates` is `true`, Inja Templates use `/` notation, i.e., `address/street` => `{ "address": { "street": "value" } }`
{{% /notice %}}

Gloo adds two additional functions that can be used within templates.

* `header` - returns the value of the specified header name, e.g., `{{ header("date") }}`
* `extraction` - returns the value of the specified extractor name, e.g. `{{ extraction("date") }}`. Only needed when
`advancedTemplates` is set to `true`, otherwise extractor values are available in the templates using their name as key

{{< highlight yaml "hl_lines=20-30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matcher:
        prefix: '/'
      routeAction:
        single:
          upstream:
            name: 'jsonplaceholder-80'
            namespace: 'gloo-system'
      routePlugins:
        transformations:
          responseTransformation:
            transformation_template:
              body:
                text: 'extractor ({{ foo }}) header ({{ header("date")}}) json ({{ phone }})'
              extractors:
                foo:
                  header: 'date'
                  regex: '\w*, (.+):(.+):(.+) GMT'
                  subgroup: 2
{{< /highlight >}}
