
---
title: "devportal.solo.iouser.proto"
---

## Package : `devportal.solo.io`



<a name="top"></a>

<a name="API Reference for user.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## user.proto


## Table of Contents
  - [UserSpec](#devportal.solo.io.UserSpec)
  - [UserSpec.BasicAuth](#devportal.solo.io.UserSpec.BasicAuth)
  - [UserStatus](#devportal.solo.io.UserStatus)







<a name="devportal.solo.io.UserSpec"></a>

### UserSpec
A User defines an entity which can authenticate to the Portal App.
Users are members of Groups, which determines which ApiDocs they can see.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| username | [string](#string) |  | The User's username must be unique within the system. |
| email | [string](#string) |  | The User's email address. |
| basicAuth | [UserSpec.BasicAuth](#devportal.solo.io.UserSpec.BasicAuth) |  | Authenticate the user with BasicAuth. |
| accessLevel | [AccessLevel](#devportal.solo.io.AccessLevel) |  | The User's access level. The user will have all the permissions granted in the access level, PLUS the permissions granted by the AccessLevels contained in the Groups to which this User belongs. |






<a name="devportal.solo.io.UserSpec.BasicAuth"></a>

### UserSpec.BasicAuth
Used for authenticating with basic auth


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| passwordSecretName | [string](#string) |  | Name of a BasicAuth secret in the cluster to use to authenticate the user. A BasicAuth secret contains the user password. This field is required. |
| passwordSecretNamespace | [string](#string) |  | Namespace containing the named BasicAuth secret. If empty, defaults to the same namespace as the user. |
| passwordSecretKey | [string](#string) |  | Name of the secret's data key which contains the password. Defaults to "password" if not set. |






<a name="devportal.solo.io.UserStatus"></a>

### UserStatus
The current status of the User. It contains information about the Portals and API docs the user has access to.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observedGeneration | [int64](#int64) |  | The observed generation of the User. When this matches the User's metadata.generation, it indicates the status is up-to-date. |
| state | [State](#devportal.solo.io.State) |  | The current state of the user |
| reason | [string](#string) |  | A human-readable string explaining the error, if any. |
| accessLevel | [AccessLevelStatus](#devportal.solo.io.AccessLevelStatus) |  | The AccessLevel currently granted to this User. |
| hasLoggedIn | [bool](#bool) |  | Special status flag that indicates whether the user has logged in for the first time. Set by the Portal Web App. |





 <!-- end messages -->

 <!-- end enums -->

 <!-- end HasExtensions -->

 <!-- end services -->

