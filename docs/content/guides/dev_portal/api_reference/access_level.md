
---
title: "access_level.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for access_level.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## access_level.proto


## Table of Contents
  - [AccessLevel](#devportal.solo.io.AccessLevel)
  - [AccessLevelStatus](#devportal.solo.io.AccessLevelStatus)







<a name="devportal.solo.io.AccessLevel"></a>

### AccessLevel
An AccessLevel defines the set of Portals and ApiDocs users or groups can access.

Users with access to a Portal will be able to log in, browse portal pages, view ApiDocs and request API Keys.

Users with access to an ApiDoc will be able to interact with that ApiDoc (e.g. view their specification,
requests Api Keys) if it is published in an accessed Portal.

AccessLevel can be defined at the User level as well as the Group level.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| portalSelector | [Selector](#devportal.solo.io.Selector) |  | Users with this access level have access to Portal objects whose labels match this selector. |
| apiDocSelector | [Selector](#devportal.solo.io.Selector) |  | Users with this access level have access to ApiDocs whose labels match this selector.<br>ApiDocs are always served from within a portal, which means an ApiDoc must appear in one of the selected Portals to be accessed. |






<a name="devportal.solo.io.AccessLevelStatus"></a>

### AccessLevelStatus
AccessLevelStatus is a status object reflecting the current status of a AccessLevel's or User's AccessLevel.
It lists the names of Portals and ApiDocs that are accessible with a given AccessLevel.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| portals | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | References to the Portals that are accessible to the User. |
| apiDocs | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | References to the ApiDocs that are accessible to the User. |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

