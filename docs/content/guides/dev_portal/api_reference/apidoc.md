
---
title: "apidoc.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for apidoc.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## apidoc.proto


## Table of Contents
  - [ApiDocSpec](#devportal.solo.io.ApiDocSpec)
  - [ApiDocSpec.OpenApi](#devportal.solo.io.ApiDocSpec.OpenApi)
  - [ApiDocStatus](#devportal.solo.io.ApiDocStatus)







<a name="devportal.solo.io.ApiDocSpec"></a>

### ApiDocSpec
An ApiDocSpec tells the DevPortal Operator how to retrieve an ApiDoc for publishing in the DevPortal UI.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dataSource | [DataSource](#devportal.solo.io.DataSource) |  | Pull the ApiDoc from this data source. |
| image | [DataSource](#devportal.solo.io.DataSource) |  | The image for this ApiDoc. |
| openApi | [ApiDocSpec.OpenApi](#devportal.solo.io.ApiDocSpec.OpenApi) |  | Set this field if the ApiDoc is of type OpenApi (default). |






<a name="devportal.solo.io.ApiDocSpec.OpenApi"></a>

### ApiDocSpec.OpenApi
Parameters specific to OpenApi ApiDoc.






<a name="devportal.solo.io.ApiDocStatus"></a>

### ApiDocStatus
The current status of the ApiDoc.
The ApiDoc will be processed as soon as one or more Portals select it for publishing.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observedGeneration | [int64](#int64) |  | The observed generation of the ApiDoc. When this matches the ApiDoc's metadata.generation, it indicates the status is up-to-date. |
| state | [State](#devportal.solo.io.State) |  | The current state of the ApiDoc. |
| reason | [string](#string) |  | A human-readable string explaining the error, if any. |
| modifiedDate | [google.protobuf.Timestamp](#google.protobuf.Timestamp) |  | Most recent date the ApiDoc was updated. |
| displayName | [string](#string) |  | User-facing display name for the ApiDoc. |
| version | [string](#string) |  | User-facing version number |
| description | [string](#string) |  | User-facing description |
| numberOfEndpoints | [int32](#int32) |  | The number of API endpoints detected in the parsed ApiDoc. |
| basePath | [string](#string) |  | The base path for making requests against this ApiDoc. |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

