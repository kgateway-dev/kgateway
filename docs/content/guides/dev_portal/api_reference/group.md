
---
title: "devportal.solo.iogroup.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for group.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## group.proto


## Table of Contents
  - [GroupSpec](#devportal.solo.io.GroupSpec)
  - [GroupStatus](#devportal.solo.io.GroupStatus)







<a name="devportal.solo.io.GroupSpec"></a>

### GroupSpec
A Group can be use to define access levels for a set of users.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| displayName | [string](#string) |  | A human-readable name for the group to display to users. |
| description | [string](#string) |  | Description for the group. |
| userSelector | [Selector](#devportal.solo.io.Selector) |  | User objects which match this selector will be considered part of this Group and have access to the  Portals and ApiDocs selected in this Group.<br>Users are always selected from the Group's own namespace. |
| accessLevel | [AccessLevel](#devportal.solo.io.AccessLevel) |  | The Groups's access level. Users in this Group will be granted access to these Portals and ApiDocs. |






<a name="devportal.solo.io.GroupStatus"></a>

### GroupStatus
the current status of the Group. It contains a list
of all the users currently selected in the group,
as well as all the ApiDocs currently selected in the group


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observedGeneration | [int64](#int64) |  | the observed generation of the Group. when this matches the Group's metadata.generation, it indicates the status is up-to-date |
| state | [State](#devportal.solo.io.State) |  | the current state of the user |
| reason | [string](#string) |  | a human-readable string explaining the error, if any |
| users | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | the User objects that are currently considered to be a part of this Group |
| accessLevel | [AccessLevelStatus](#devportal.solo.io.AccessLevelStatus) |  | the AccessLevel currently granted to to members of this group |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

