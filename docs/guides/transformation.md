# Transformation

## Glossary

- Classic Transformation: This is the C++ implementation that's statically link to a custom built Envoy.
- RustFormation: This is written in rust using Envoy Dynamic Module. The module is bundled into the envoy_wrapper image together with the Envoy binary. It's loaded at run time when Envoy starts up.

## Overview

The Transformation under TrafficPolicy supports modifying request/response in the headers or the body. It also supports Inja templating syntax. Full documentation can be found [here](https://kgateway.dev/docs/envoy/latest/traffic-management/transformations/).

## Implementations

Before v2.2.0, the Classic Transformation is the default implementation. With the release of v2.2.0, we switched to Rustformation as the default implementation. However, there are some differences between the two implementations because of the underlying Inja templating library used.

Here are the differences between [minijinja](https://github.com/mitsuhiko/minijinja) in rust vs the [inja](https://github.com/pantor/inja) lib we use in C++:

| | C++ | Rust | Note |
| -- | -- | -- | -- |
| White Spaces | Leave all white spaces alone | Right trim whitespace at the end | |
| replace_with_random | Each random pattern is generated once per request for a specific string to replace. For example, replace_with_random(“abc”, “a”) and replace_with_random(“cba”, “a”) will replace “a” with the same pattern within the same request context | Each random pattern is generated per function call. So, with the same example, “a” will be replaced with different random pattern | |
| Default body parsing | Default to parse as Json as long as any transformation is enabled regardless of if you actually use the body or not | Default to parse as String (no parsing). | |
| Body parsedAsJson accessor syntax | level_{%- if headers != \"\" -%}{{headers.X-Incoming-Stuff.0}}{% else %}unknown{% endif %} | level_{%- if headers != \"\" -%}{{headers[\"X-Incoming-Stuff\"][0]}}{% else %}unknown{% endif %}" | There are 2 differences:the .0 notation to access item 0 is not supported in the rust minijinja lib, it needs to be [0] but worse, the C++ implementation ONLY supports .0 and fail the config validation if [0] is used when the variable name contains - , the [] notation has to be used in minijinja. So, it needs to be headers["X-Incoming-Stuff"] but again, the C++ implementation doesn't support that. |
| Json field name and custom function name collision | C++ doesn’t have this problem. | When we have the body parseAsJson set, all the json fields are put into the template context so they can be accessed directly. However, if a field name is the same as one of our [custom function names](https://kgateway.dev/docs/envoy/latest/traffic-management/transformations/templating-language/#custom-inja-functions), the rust minijinja template parsing seems to prefer to resolve it to the json value. | For example, we have a custom function  context() but if the json body also has a field named context, the template rendering  will resolve context() to the json value from the body and complain it’s not callable. |
| Body buffering | The C++ Transformation filter buffers the data inside it's own structure and does it's own buffer limit detection using Envoy's decoder buffer limit setting | Rely on Envoy to do the buffering before we process the entire body | |
| Adding Multiple Headers with the same name |   |   | The kgateway  transformation API defines the list of header as a map. So, you cannot add the same header more than once even though the backend supports it. This behavior is the same in kgateway for both the classic and rust transformation as this is an API limit but just want to point this out. |

## Strict Mode Validation

Strict Mode Validation is not supported yet with Rustformation. This is due to build complexity to include the dynamic module in the control plane image. This will be addressed in future updates.

If Strict Mode Validation is needed, see the [Classic Transformation Deprecation](#classic-transformation-deprecation) section below to switch back to Classic Transformation on x86 architecture.

## Initial arm64 support

Starting from v2.2.0, we supports building on arm64 architecture and uses Envoy arm64 binary directly from upstream Envoy container.

### add header function not supported with Rustformation on arm64

The rust transformation dynamic module is building with v1.36 Envoy and unfortunately, the add header function has not been exposed in the rust sdk, so on arm64 build, the add header function in Transformation is a no-op. Please use set header instead. This will be addressed when Envoy is upgraded to v1.37 in the future.

The add header function works correctly on x86 architecture because we are still using a custom build Envoy binary and we have patched the rust sdk there to support add header.

## Classic Transformation Deprecation

Starting from v2.2.0, Classic Transformation is being deprecated and will be removed in future release. On x86_64 architecture, this release is still using the custom envoy build, so it is possible to switch back to the Classic Transformation if needed by using the helm settings `controller.extraEnv.KGW_USE_RUST_FORMATIONS=false`.