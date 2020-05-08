
---
title: "common.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for common.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## common.proto


## Table of Contents
  - [DataSource](#devportal.solo.io.DataSource)
  - [DataSource.ConfigMapData](#devportal.solo.io.DataSource.ConfigMapData)
  - [ObjectRef](#devportal.solo.io.ObjectRef)
  - [Selector](#devportal.solo.io.Selector)
  - [Selector.MatchLabelsEntry](#devportal.solo.io.Selector.MatchLabelsEntry)

  - [State](#devportal.solo.io.State)






<a name="devportal.solo.io.DataSource"></a>

### DataSource
Source of binary data.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inlineString | [string](#string) |  | Data is stored as an inline string. |
| inlineBytes | [bytes](#bytes) |  | Data is stored as an array of bytes. |
| fetchUrl | [string](#string) |  | Data is stored as a URL. |
| configMap | [DataSource.ConfigMapData](#devportal.solo.io.DataSource.ConfigMapData) |  | Data is stored in a ConfigMap. |






<a name="devportal.solo.io.DataSource.ConfigMapData"></a>

### DataSource.ConfigMapData
Data stored in a ConfigMap Data map entry


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the config map. |
| namespace | [string](#string) |  | The namespace of the config map. |
| key | [string](#string) |  | The name of the key in the ConfigMap's data map. |






<a name="devportal.solo.io.ObjectRef"></a>

### ObjectRef
A reference to an object.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the object. |
| namespace | [string](#string) |  | Namespace of the object. |






<a name="devportal.solo.io.Selector"></a>

### Selector
Used to select other resources.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| matchLabels | [][Selector.MatchLabelsEntry](#devportal.solo.io.Selector.MatchLabelsEntry) | repeated | Select only resources that match the given label set. |






<a name="devportal.solo.io.Selector.MatchLabelsEntry"></a>

### Selector.MatchLabelsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |





 <!-- end messages -->


<a name="devportal.solo.io.State"></a>

### State
The State of a Dev-Portal object

| Name | Number | Description |
| ---- | ------ | ----------- |
| Pending | 0 | Waiting to be processed. |
| Processing | 1 | Currently processing. |
| Invalid | 2 | Invalid parameters supplied, will not continue. |
| Failed | 3 | Failed during processing. |
| Succeeded | 4 | Finished processing successfully. |


 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

