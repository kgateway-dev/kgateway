---
title: Transformations
weight: 10
description: Use the Gloo Edge Transformation API to transform requests and responses
---

One of the core features of any API Gateway is the ability to transform the traffic that it manages. To really enable the decoupling of your services, the API Gateway should be able to mutate requests before forwarding them to your upstream services and do the same with the resulting responses before they reach the downstream clients. Gloo Edge delivers on this promise by providing you with a powerful transformation API.

To review where transformations happen as Gloo Edge filters traffic, see [Traffic processing]({{% versioned_link_path fromRoot="/introduction/traffic_filter/" %}}).

## Defining a transformation
Transformations are defined by adding the `transformations` attribute to your Virtual Services. You can define this attribute on three different Virtual Service sub-resources:
 
- **VirtualHosts**
- **Routes**
- **WeightedDestinations**

The configuration format is the same in all three cases and must be specified under the relevant `options` attribute. For example, to configure transformations for all traffic matching a Virtual Host, you need to add the following attribute to your Virtual Host definition:

{{< highlight yaml "hl_lines=3-11" >}}
# This snippet has been abridged for brevity
virtualHost:
  options:
    stagedTransformations:
      regular:
        requestTransforms:
        - requestTransformation:
            transformationTemplate:
              headers:
                foo:
                  text: 'bar'
{{< /highlight >}}

#### Inheritance rules

By default, a transformation defined on a Virtual Service attribute is **inherited** by all the child attributes:

- transformations defined on `VirtualHosts` are inherited by `Route`s and `WeightedDestination`s.
- transformations defined on `Route`s are inherited by `WeightedDestination`s.

If a child attribute defines its own transformation, it overrides the configuration on its parent.
However, if `inheritTransformation` is set to true on the `stagedTransformations` for a Route, it can inherit transformations
from its parent as illustrated below.

Let's define the `virtualHost` and its child route is defined as follows:
{{< highlight yaml "hl_lines=7-13 20-27" >}}
# This snippet has been abridged for brevity, and only includes transformation-relevant config
virtualHost:
  options:
    stagedTransformations:
      regular:
        requestTransforms:
        - matchers:
            - prefix: '/parent'
          requestTransformation:
            transformationTemplate:
              headers:
                foo:
                  text: 'bar'
  routes:
    - options:
        stagedTransformations:
          inheritTransformation: true
          regular:
            requestTransforms:
            - matchers:
              - prefix: '/child'
              requestTransformation:
                transformationTemplate:
                  headers:
                    foo:
                      text: 'baz'
{{< /highlight >}}

Because `inheritTransformation` is set to `true` on the child route, the parent `virtualHost` transformation config is merged into the child. The child route's transformations look like the following.

{{< highlight yaml "hl_lines=8-22" >}}
# This snippet has been abridged for brevity, and only includes transformation-relevant config
routes:
- options:
  stagedTransformations:
    inheritTransformation: true
    regular:
      requestTransforms:
      - matchers:
        - prefix: '/child'
        requestTransformation:
          transformationTemplate:
            headers:
              foo:
                text: 'baz'
      - matchers:
        - prefix: '/parent'
        requestTransformation:
          transformationTemplate:
            headers:
              foo:
                text: 'bar'
{{< /highlight >}}

As stated above, the route's configuration overrides its parent's, but now it also inherits the parent's transformations. So in this case,
routes matching `/parent` are also transformed. If `inheritTransformation` were set to `false`, the matching `/parent` routes would not be transformed. 
Note that only the first matched transformation runs, so if both the child and the parent had the same matchers, the child's transformation would run.

### Configuration format
In this section we will detail all the properties of the `stagedTransformations` {{< protobuf display="object" name="transformation.options.gloo.solo.io.TransformationStages" >}},
which has the following structure:

```yaml
stagedTransformations:
  early:
    # early transformations
  regular: 
    requestTransforms:
      - matcher:
          prefix : '/'
        clearRouteCache: bool
        requestTransformation: {}
        responseTransformation: {}
    responseTransforms: {}
  inheritTransformations: bool
```

The `early` and `regular` attributes are used to specify when in the envoy filter chain the transformations run. The following diagram illustrates the stages at which each of the transformation filters run in relation to other envoy filters. 

![Transformation Filter Stages]({{% versioned_link_path fromRoot="/img/transformation_stages.png" %}})

The `inheritTransformation` attribute allows child routes to inherit transformations from their parent RouteTables and/or VirtualHosts. This is detailed in the Inheritance rules section.

The `requestTransforms` attribute specifies a list of transformations which will be evaluated based on the request attributes. The first transformation which has a `matcher` that matches the request attributes will run.

The `responseTransforms` attribute specifies a list of transformations which will be evaluated based on the response attributes. A transformation will only be chosen from this list if no transformation in `requestTransform` matched the request.

The `clearRouteCache` attribute is a boolean value that determines whether the route cache should be cleared if the request transformation was applied. If the transformation modifies the headers in a way that affects routing, this attribute must be set to `true`. The default value is `false`.

The `requestTransformation` and `responseTransformation` attributes have the {{< protobuf display="same format" name="transformation.options.gloo.solo.io.Transformation" >}} and specify transformations that will be applied to requests and responses respectively. The format can take one of two forms:

- `headerBodyTransform`: this type of transformation will make all the headers available in the body. The result will be a JSON body that consists of two attributes: `headers`, containing the headers, and `body`, containing the original body.
- `transformationTemplate`: this type of transformation allows you to define transformation templates. This is the more powerful and flexible type of transformation. We will spend the rest of this guide to describe its properties.

#### Transformation templates
{{< protobuf display="Templates" name="envoy.api.v2.filter.http.TransformationTemplate" >}} are the core of Gloo Edge's transformation API. They allow you to mutate the headers and bodies of requests and responses based on the properties of the headers and bodies themselves. The following snippet illustrates the structure of the `transformationTemplate` object:

```yaml
transformationTemplate:
  parseBodyBehavior: {}
  ignoreErrorOnParse: bool
  extractors:  {}
  headers: {}
  # Only one of body, passthrough, and mergeExtractorsToBody can be specified
  body: {} 
  passthrough: {}
  mergeExtractorsToBody: {}
  dynamicMetadataValues: []
  advancedTemplates: bool
```

{{% notice note %}}
The `body`, `passthrough`, and `mergeExtractorsToBody` attributes define three different ways of handling the body of the request/response. Please note that **only one of them may be set**, otherwise Gloo Edge will reject the `transformationTemplate`.
{{% /notice %}}

Let's go ahead and describe each one of these attributes in detail.

##### parseBodyBehavior
This attribute determines how the request/response body will be parsed and can have one of two values:

- `ParseAsJson`: Gloo Edge will attempt to parse the body as a JSON structure. *This is the default behavior*.
- `DontParse`: the body will be treated as plain text.

The important part to know about the `DontParse` setting is that the body will be buffered and available, but will not be parsed. If you're looking to skip any body buffering completely, see the section [on passthrough: {}](#passthrough)

As we will [see later](#templating-language), some of the templating features won't be available when treating the body as plain text.

##### ignoreErrorOnParse
By default, Gloo Edge will attempt to parse the body as JSON, unless you have `DontParse` set as the `parseBodyBehavior`. If `ignoreErrorOnParse` is set to `true`, Envoy will not throw an exception in case the body parsing fails. Defaults to `false`.

Implicit in this setting is that the body will be buffered and available. If you're looking to skip any body buffering completely, see the section [on passthrough: {}](#passthrough)

##### extractors
Use this attribute to extract information from a request or response. It consists of a set of mappings from a string to an `extraction`: 

- the `extraction` defines which information will be extracted
- the string key will provide the extractor with a name it can be referred by.

An extraction must have one of two sources:

- `header`: extract information from a header with the given name.
    ```yaml
    extractors:
      myExtractor:
        header: 'foo'
        regex: '.*'
    ```
- `body`: extract information from the body. This attribute takes an empty value (as there is always only one body).
    ```yaml
    extractors:
      myExtractor:
        body: {}
        regex: '.*'
    ```

{{% notice note %}}
The `body` extraction source has been introduced with **Gloo Edge**, release 0.20.12, and **Gloo Edge Enterprise**, release 0.20.7. If you are using an earlier version, it will not work.
{{% /notice %}}

Extracting the body is generally not useful when Gloo Edge has already parsed it as JSON, the default behavior. The parsed body data can be directly referenced using standard JSON syntax. The `body` extractor treats the body as plaintext, and is interpreted using a regular expression as noted below. This can be useful for body data that cannot be parsed as JSON.

An extraction must also define which information is to be extracted from the source. This can be done by providing a regular expression via the `regex` attribute. The regular expression will be applied to the body or to the value of relevant header and must match the entire source. If your regular expression uses _capturing groups_, you can select the group match you want to use via the `subgroup` attribute.

{{% notice note %}}
The `regex` attribute is required when specifying an extraction. Even if a `regex` attribute is not required to capture a subset of the source information, you must still specify a catch-all pattern like `regex: '.*'`. If no `regex` is specified, the extractor will silently fail.
The `regex` **must match the entire source** even if you are looking to extract a subset of the source information in a capturing group. If the regex doesn't match the entire input, the regex will silently fail.
{{% /notice %}}

As an example, to define an extraction named `foo` which will contain the value of the `foo` query parameter you can apply the following configuration:

```yaml
extractors:
  # This is the name of the extraction
  foo:
    # The :path pseudo-header contains the URI
    header: ':path'
    # Use a nested capturing group to extract the query param
    regex: '(.*foo=([^&]*).*)'
    # Select the second group match
    subgroup: 2
```

Another example shows how to extract a value from the body using a regex.
If the body is:

```text
Here is a body containing the identification number of the object.

id: 4423
```
and we want to extract the id number, we can define an extractor:
```yaml
extractors:
  # This is the name of the extraction
  bodyIdExtractor:
    # This empty struct must be included to extract from the body
    body: {}
    # The regex must match the ENTIRE body, else the extraction will not work
    # The [\s\S]* allows us to match any character, making the regex match the entire source.
    regex: '[\s\S]*id: ([0-9]{4})[\s\S]*'
    # Select the first subgroup match ([0-9]{4}) since we only want the id number
    subgroup: 1
```

The result will be the `bodyIdExtractor` having value `4423`.

Extracted values can be used in two ways:

- You can reference extractors by their name in template strings, e.g. `{{ my-extractor }}` (or `{{ extraction(my-extractor) }}`, if you are setting `advancedTemplates` to `true`) will render to the value of the `my-extractor` extractor.
- You can use them in conjunction with the `mergeExtractorsToBody` body transformation type to merge them into the body.

##### headers
Use this attribute to apply templates to request/response headers. It consists of a map where each key determines the name of the resulting header, while the corresponding value is a {{< protobuf display="template" name="envoy.api.v2.filter.http.InjaTemplate" >}} which will determine the value.

For example, to set the header `foo` to the value of the header `bar`, you could use the following configuration:

```yaml
transformationTemplate:
  headers:
    foo:
      text: '{{ header("bar") }}'
```

You could also use the parsed data from the body and add information to the header. For instance, given a request body:

```json
{
  "Name": "Gloo",
  "Favorites": {
    "Color": "Blue"
  }
}
```

You could reference the value stored in `Color` and place it into the headers like so:

```yaml
transformationTemplate:
  headers:
    color:
      text: '{{ Favorites.Color }}'
```

See the [template language section](#templating-language) for more details about template strings.

##### body
Use this attribute to apply templates to the request/response body. It consists of a template string that will determine the content of the resulting body.

As an example, the following configuration snippet could be used to transform a response body only if the HTTP response code is `404` and preserve the original response body in other cases.

```yaml
transformationTemplate:
  # [...]
  body: 
    text: '{% if header(":status") == "404" %}{ "error": "Not found!" }{% else %}{{ body() }}{% endif %}'
  # [...]
```

See the [template language section](#templating-language) for more details about template strings.

##### passthrough
In some cases your do not need to transform the request/response body nor extract information from it. In these cases, particularly if the payload is large, you should use the `passthrough` attribute to instruct Gloo Edge to ignore the body (i.e. to not buffer it). The attribute always takes just the empty value:

```yaml
transformationTemplate:
  # [...]
  passthrough: {}
  # [...]
```

If you're looking to parse the body, and either [ignore errors on parsing](#ignoreerroronparse), or just [disable JSON parsing](#parsebodybehavior), see those sections in this document, respectively. 

##### mergeExtractorsToBody
Use this type of body transformation to merge all the `extractions` defined in the `transformationTemplate` to the body. The values of the extractions will be merged to a location in the body JSON determined by their names. You can use separators in the extractor names to nest elements inside the body.

For an example, see the following configuration:

```yaml
transformationTemplate:
  mergeExtractorsToBody: {}
  extractors:
  path:
    header: ':path'
    regex: '.*'
  # The name of this attribute determines where the value will be nested in the body
  host.name:
    header: 'host'
    regex: '.*'
```

This will cause the resulting body to include the following extra attributes (in additional to the original ones):

```json
{
  "path": "/the/request/path",
  "host": {
    "name": "value of the 'host' header"
  }
}
```

##### dynamicMetadataValues
This attribute can be used to define an [Envoy Dynamic Metadata](https://www.envoyproxy.io/docs/envoy/latest/configuration/advanced/well_known_dynamic_metadata) entry. This metadata can be used by other filters in the filter chain to implement custom behavior.

As an example, the following configuration creates a dynamic metadata entry in the `com.example` namespace with key  `foo` and value equal to that of the `foo` header . 

```yaml
dynamicMetadataValues:
- metadataNamespace: "com.example"
  key: 'foo'
  value:
    text: '{{ header("foo") }}'

```

The `metadataNamespace` is optional. It defaults to the namespace of the Gloo Edge transformation filter name, i.e. `io.solo.transformation`.

A common use case for this attribute is to define custom data to be included in your access logs. See the [dedicated tutorial]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/enrich_access_logs/" %}}) for an example of how this can be achieved.

##### advancedTemplates
This attribute determines which notation to use when accessing elements in JSON structures. If set to `true`, Gloo Edge will expect JSON pointer notation (e.g. "time/start") instead of dot notation (e.g. "time.start"). Defaults to `false`.

Please note that, if set to `true`, you will need to use the `extraction` function to access extractors in template strings (e.g. `{{ extraction("myExtractor") }}`); if the default value of `false` is used, extractors will simply be available by their name (e.g. `{{ myExtractor }}`).

#### Templating language
{{% notice note %}}
Templates can be used only if the request/response payload is a JSON string.
{{% /notice %}}

Gloo Edge templates are powered by the [Inja](https://github.com/pantor/inja) template engine, which is inspired by the popular [Jinja](https://palletsprojects.com/p/jinja/) templating language in Python. When writing your templates, you can take advantage of all the core _Inja_ features, i.a. loops, conditional logic, and functions.

In addition to the standard functions available in the core _Inja_ library, you can use additional custom functions that we have added:

- `header(header_name)`: returns the value of the header with the given name.
- `extraction(extractor_name)`: returns the value of the extractor with the given name.
- `env(env_var_name)`: returns the value of the environment variable with the given name.
- `body()`: returns the request/response body.
- `context()`: returns the base JSON context (allowing for example to range on a JSON body that is an array).
- `request_header(header_name)`: return the value of the request header with the given name. Useful for including request header values in response transformations.
- `base64_encode(string)`: encodes the input string to base64.
- `base64_decode(string)`: decodes the input string from base64.
- `substring(string, start_pos, substring_len)`: returns a substring of the input string, starting at `start_pos` and extending for `substring_len` characters. If no `substring_len` is provided or `substring_len` is <= 0, the substring extends to the end of the input string.

You can use templates to mutate [headers](#headers), the [body](#body), and [dynamic metadata](#dynamicmetadatavalues).

#### XSLT Transformation
{{% notice note %}}
The XSLT transformation has been introduced with **Gloo Edge Enterprise**, release v1.8.0-beta3. If you are using an earlier version, it will not work.
{{% /notice %}}
{{< protobuf display="XSLT transformations" name="envoy.config.transformer.xslt.v2.XsltTransformation" >}} allow transformations on HTTP requests using the XSLT transformation language.
The following snippet illustrates the structure of the `xsltTransformation` object.
```yaml
xsltTransformation:
  xslt: string
  setContentType: string
  nonXmlTransform: bool
```

##### xslt
The XSLT transformation is specified in this field as a string. Like other transformations, an invalid XSLT transformation will not be accepted and envoy 
validation will reject the transformation configuration.

##### setContentType
XSLT transformations can be used to transform HTTP body between content type. For example, from [XML to JSON](https://www.w3.org/TR/xslt-30/#func-xml-to-json), or [JSON to XML](https://www.w3.org/TR/xslt-30/#func-json-to-xml).
In the case of these transformations, the `content-type` HTTP header is set to the value of `setContentType`. If left empty, the `content-type` header is unchanged.

##### nonXmlTransform
XSLT transformations typically accept only XML as input. If the input to the transformation is not XML, this should be set to true. For example, if 
the XSLT transformation is transforming a JSON input to XML, this would be set to `true`. By default, this is false and the XSLT transformation will only accept XML input.

### Common use cases
On this page we have seen all the properties of the Gloo Edge Transformation API as well as some simple example snippets. If are looking for complete examples, please check out the following tutorials, which will guide you through some of the most common transformation use cases.

{{% children description="true" %}}
