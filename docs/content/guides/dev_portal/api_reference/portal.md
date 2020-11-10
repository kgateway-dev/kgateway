
---
title: "portal.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for portal.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## portal.proto


## Table of Contents
  - [CustomStyling](#devportal.solo.io.CustomStyling)
  - [KeyScope](#devportal.solo.io.KeyScope)
  - [KeyScopeStatus](#devportal.solo.io.KeyScopeStatus)
  - [PortalSpec](#devportal.solo.io.PortalSpec)
  - [PortalStatus](#devportal.solo.io.PortalStatus)
  - [StaticPage](#devportal.solo.io.StaticPage)







<a name="devportal.solo.io.CustomStyling"></a>

### CustomStyling
Custom Styling options for a portal


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| primaryColor | [string](#string) |  |  |
| secondaryColor | [string](#string) |  |  |
| backgroundColor | [string](#string) |  |  |
| navigationLinksColorOverride | [string](#string) |  |  |
| buttonColorOverride | [string](#string) |  |  |
| defaultTextColor | [string](#string) |  |  |






<a name="devportal.solo.io.KeyScope"></a>

### KeyScope
A KeyScope defines the scope (set of accessible ApiDocs) of an Api Key provisioned for the Portal.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the scope. This field is required and must be a unique, DNS-compliant string. The name of the key scope will determine how the API key secrets will be labeled. Each API key secret generated for this key scope will include a label with key `portals.devportal.solo.io/<PORTAL_NS>.<PORTAL_NAME>.<KEY_SCOPE_NAME>` and value equal to "true". |
| namespace | [string](#string) |  | The namespace in which the ApiKeys will be created for this key scope will be created. If empty, defaults to the namespace of the Portal |
| displayName | [string](#string) |  | Optional display name for the key scope. If empty, portals will display the key scope name. |
| description | [string](#string) |  | Description of the key scope. |
| apiDocs | [Selector](#devportal.solo.io.Selector) |  | Create ApiKeys to access ApiDocs matching these labels. Can only include ApiDocs published to the Portal.<br>If the Group ApiDocs and Portal ApiDocs do not intersect, the user will see no ApiDocs. |






<a name="devportal.solo.io.KeyScopeStatus"></a>

### KeyScopeStatus
Gives the Status of a KeyScope that lives on the portal.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the scope |
| accessibleApiDocs | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | The API docs that can be accessed with this key scope. |
| provisionedKeys | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | The ApiKey Secrets that have been provisioned for this key scope. |






<a name="devportal.solo.io.PortalSpec"></a>

### PortalSpec
A PortalSpec tells the Gloo Portal Operator to fetch and serve static assets which are used by the Gloo Portal UI.
Each portal can publish one or more ApiDocs.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| displayName | [string](#string) |  | Display name of the portal. |
| description | [string](#string) |  | Description for the portal. |
| domains | [][string](#string) | repeated | The domains on which this Portal will be served.  The Host header received by the Portal Web App will be matched to one of these domains in order to determine which Portal will be served.<br>To prevent undefined behavior, creating a Portal whose domain conflicts with an existing Portal will result in the Portal resource being placed into an 'Invalid' state. |
| primaryLogo | [DataSource](#devportal.solo.io.DataSource) |  | Logo to display on the portal. |
| favicon | [DataSource](#devportal.solo.io.DataSource) |  | Browser favicon for the portal. |
| banner | [DataSource](#devportal.solo.io.DataSource) |  | The banner image for the portal. |
| customStyling | [CustomStyling](#devportal.solo.io.CustomStyling) |  | Custom Styling overrides. |
| staticPages | [][StaticPage](#devportal.solo.io.StaticPage) | repeated | Static markdown content pages for the portal. |
| publishApiDocs | [Selector](#devportal.solo.io.Selector) |  | Select ApiDocs matching these labels for publishing on the Portal. ApiDocs are always selected from the Portal's own namespace.<br>The set of ApiDocs a specific user sees upon login will be filtered by the permissions associated either with that user's AccessLevel, or with the AccessLevels of the groups that the user is a member of.<br>If the User's accessible ApiDocs and Portal ApiDocs do not intersect, the user will see no ApiDocs. |
| keyScopes | [][KeyScope](#devportal.solo.io.KeyScope) | repeated | KeyScopes define available scopes for provisioning Api Keys to work with the ApiDocs published on this portal |






<a name="devportal.solo.io.PortalStatus"></a>

### PortalStatus
The current status of the Portal. The Portal will be processed as soon as it is created in the cluster.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observedGeneration | [int64](#int64) |  | The observed generation of the Portal. When this matches the Portal's metadata.generation, it indicates the status is up-to-date. |
| state | [State](#devportal.solo.io.State) |  | The current state of the portal. |
| reason | [string](#string) |  | A human-readable string explaining the error, if any. |
| publishUrl | [string](#string) |  | The published URL at which the portal can be accessed |
| apiDocs | [][ObjectRef](#devportal.solo.io.ObjectRef) | repeated | The ApiDocs that are currently considered to be part of this Portal. |
| keyScopes | [][KeyScopeStatus](#devportal.solo.io.KeyScopeStatus) | repeated | Status info for each KeyScope |






<a name="devportal.solo.io.StaticPage"></a>

### StaticPage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | The name of the page. |
| description | [string](#string) |  | Description of the page. |
| path | [string](#string) |  | The path for this page relative to the portal base URL. |
| navigationLinkName | [string](#string) |  | The name of the link displayed on the portal navigation bar. |
| displayOnHomepage | [bool](#bool) |  | Set to true if you want to display a tile that links to the static page on the portal home page. Only one of the static pages for a portal can set this flag to true. |
| content | [DataSource](#devportal.solo.io.DataSource) |  | Markdown content for the page. |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

