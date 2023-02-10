---
title: LDAP
weight: 60
description: Authenticate and authorize requests using LDAP.
---

{{% notice note %}}
The LDAP feature was introduced with **Gloo Edge Enterprise**, release 0.18.27. If you are using an earlier 
version, this tutorial will not work.
{{% /notice %}}

The _Lightweight Directory Access Protocol_, commonly referred to as LDAP, is an open protocol used to store and retrieve 
hierarchically structured data over a network. It has been widely adopted by enterprises to centrally store and secure 
organizational information. A common use case for LDAP is to maintain information about members of an organization, 
assign them to specific user groups, and give each of them access to resources based on their group memberships.

In this guide you deploy a simple LDAP server to your Kubernetes cluster to explore how you can use it together with 
Gloo Edge to authenticate users and control access to a target service based on the user's group memberships.

{{% notice tip %}}
Check out [**this excellent tutorial**](https://www.digitalocean.com/community/tutorials/understanding-the-ldap-protocol-data-hierarchy-and-entry-components) by Digital Ocean to familiarize yourself with the basic concepts and components of an LDAP server; although it is not 
strictly necessary, it will help you better understand this guide.
{{% /notice %}}


## Prerequisites
{{< readfile file="/static/content/setup_notes" markdown="true">}}


## Step 1: Create a simple Virtual Service
Let's start by creating a simple service that returns "Hello World" when receiving HTTP requests:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: http-echo
  name: http-echo
spec:
  selector:
    matchLabels:
      app: http-echo
  replicas: 1
  template:
    metadata:
      labels:
        app: http-echo
    spec:
      containers:
      - image: hashicorp/http-echo:latest
        name: http-echo
        args: ["-text='Hello World!'"]
        ports:
        - containerPort: 5678
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: http-echo
  labels:
    service: http-echo
spec:
  ports:
  - port: 5678
    protocol: TCP
  selector:
    app: http-echo
EOF
```

Now we can create a Virtual Service that routes any requests with the `/echo` prefix to the `http-echo` service.

{{< highlight shell "hl_lines=17-21" >}}
{{< readfile file="guides/security/auth/extauth/ldap/vs-echo-no-auth.sh">}}
{{< /highlight >}}


To verify that the Virtual Service works, let's send a request to `/echo`:

```bash
curl $(glooctl proxy url)/echo
```

returns
```
'Hello World!'
```

## Step 2: Deploy an LDAP server
We also need to deploy an LDAP server to your cluster and configure it with a simple set of users and groups. This 
information is used to determine which requests can access the upstream that you created earlier. 

We have prepared a [**shell script**](setup-ldap.sh) that takes care of setting up the necessary resources. It creates:

1. a `configmap` with the LDAP server bootstrap configuration
2. a `deployment` running OpenLDAP
3. a `service` fronting the deployment
 
The script accepts an optional string argument, which determines the namespace in which the resources are created. If no namespace is provided, the resources are created in the `default` namespace. After you have downloaded the script to your working directory, you can run the following 
commands to execute it:

```shell
chmod +x setup-ldap.sh
./setup-ldap.sh    

No namespace provided, using default namespace
Creating configmap with LDAP server bootstrap config...
configmap/ldap created
Creating LDAP service and deployment...
deployment.apps/ldap created
service/ldap created
```

{{% expand "The details of the script are beyond the scope of this guide; if you are interested, you can inspect them by clicking on this paragraph." %}}
```bash
{{< readfile file="guides/security/auth/extauth/ldap/setup-ldap.sh" >}}
```
{{% /expand %}}

To understand the user configuration, it is worth looking at the last two data entries in the config map:

```text
03_people.ldif: |
  # Create a parent 'people' entry
  dn: ou=people,dc=solo,dc=io
  objectClass: organizationalUnit
  ou: people
  description: All solo.io people

  # Add 'marco'
  dn: uid=marco,ou=people,dc=solo,dc=io
  objectClass: inetOrgPerson
  cn: Marco Schmidt
  sn: Schmidt
  uid: marco
  userPassword: marcopwd
  mail: marco.schmidt@solo.io

  # Add 'rick'
  dn: uid=rick,ou=people,dc=solo,dc=io
  objectClass: inetOrgPerson
  cn: Rick Ducott
  sn: Ducott
  uid: rick
  userPassword: rickpwd
  mail: rick.ducott@solo.io

  # Add 'scottc'
  dn: uid=scottc,ou=people,dc=solo,dc=io
  objectClass: inetOrgPerson
  cn: Scott Cranton
  sn: Cranton
  uid: scottc
  userPassword: scottcpwd
  mail: scott.cranton@solo.io
04_groups.ldif: |+
  # Create top level 'group' entry
  dn: ou=groups,dc=solo,dc=io
  objectClass: organizationalUnit
  ou: groups
  description: Generic parent entry for groups

  # Create the 'developers' entry under 'groups'
  dn: cn=developers,ou=groups,dc=solo,dc=io
  objectClass: groupOfNames
  cn: developers
  description: Developers group
  member: uid=marco,ou=people,dc=solo,dc=io
  member: uid=rick,ou=people,dc=solo,dc=io
  member: uid=scottc,ou=people,dc=solo,dc=io

  # Create the 'sales' entry under 'groups'
  dn: cn=sales,ou=groups,dc=solo,dc=io
  objectClass: groupOfNames
  cn: sales
  description: Sales group
  member: uid=scottc,ou=people,dc=solo,dc=io

  # Create the 'managers' entry under 'groups'
  dn: cn=managers,ou=groups,dc=solo,dc=io
  objectClass: groupOfNames
  cn: managers
  description: Managers group
  member: uid=rick,ou=people,dc=solo,dc=io
```

We can see that the root of the LDAP directory hierarchy is the `dc=solo,dc=io` entry, which has two child entries:

- `ou=groups,dc=solo,dc=io` is the parent entry for user groups in the organization. It contains three groups:
    - cn=`developers`,ou=groups,dc=solo,dc=io
    - cn=`sales`,ou=groups,dc=solo,dc=io
    - cn=`managers`,ou=groups,dc=solo,dc=io
    
- `ou=people,dc=solo,dc=io` is the parent entry for people in the organization and in turn has the following entries:
   - uid=`marco`,ou=people,dc=solo,dc=io
   - uid=`rick`,ou=people,dc=solo,dc=io
   - uid=`scott`,ou=people,dc=solo,dc=io
   
The user credentials and memberships are summarized in the following table:

|  username |   password   | member of developers | member of sales | member of managers |
|-----------|--------------|----------------------|-----------------|--------------------|
| marco     | marcopwd     | Y                    |  N              |   N               |
| rick      | rickpwd      | Y                    |  N              |   Y               |
| scott     | scottpwd     | Y                    |  Y              |   N               |

To test that the LDAP server has been correctly deployed, let's port-forward the corresponding deployment:

```bash
kubectl port-forward deployment/ldap 8088:389
```

In a different terminal instance, run the following command (you must have `ldapsearch` installed):

```bash
ldapsearch -H ldap://localhost:8088 -D "cn=admin,dc=solo,dc=io" -w "solopwd" -b "dc=solo,dc=io" -LLL dn
```

You should see the following output, listing the **distinguished names (DNs)** of all entries located in the subtree 
rooted at `dc=solo,dc=io`:

```text
dn: dc=solo,dc=io

dn: cn=admin,dc=solo,dc=io

dn: ou=people,dc=solo,dc=io

dn: uid=marco,ou=people,dc=solo,dc=io

dn: uid=rick,ou=people,dc=solo,dc=io

dn: uid=scottc,ou=people,dc=solo,dc=io

dn: ou=groups,dc=solo,dc=io

dn: cn=developers,ou=groups,dc=solo,dc=io

dn: cn=sales,ou=groups,dc=solo,dc=io

dn: cn=managers,ou=groups,dc=solo,dc=io
```

## Step 3: Set up LDAP authentication for the Virtual Service
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

Now that we have all the necessary components in place, let's use the LDAP server to secure the Virtual Service we created 
earlier. 

### LDAP auth flow 
Before updating our Virtual Service, it is important to understand how Gloo Edge interacts with the LDAP server. Let's first 
look at the {{< protobuf
display="LDAP auth configuration"
name="enterprise.gloo.solo.io.Ldap"
>}}:

- `address`: The address of the LDAP server that Gloo Edge will query when a request matches the Virtual Service.
- `userDnTemplate`: A template string that Gloo Edge uses to build the DNs of the user entry or service account that 
   needs to be authenticated and authorized. It must contain a single occurrence of the “%s” placeholder.
- `membershipAttributeName`: The case-insensitive name of the attribute that contains the names of the groups an entry is a
   member of. Defaults to `memberOf` if not provided.
- `allowedGroups`: The DNs of the user groups that are allowed to access the secured upstream.
- `searchFilter`: The filter to use when searching for the user entry that you want to authorize.
- `disableGroupChecking`: If set to true, disables validation for the membership attribute of the user entry. 
- `groupLookupSettings`: Configures a service account to look up group memberships from the LDAP server. The service account must be set up in the LDAP server. 

To better understand how this configuration is used, let's go over the steps that Gloo Edge performs when it detects a 
request that needs to be authenticated with LDAP:

1. Look for a [Basic Authentication](https://en.wikipedia.org/wiki/Basic_access_authentication) header on the request 
   and extract the username and credentials. 
2. If the header is not present, return a `401` response. 
3. Try to perform a [BIND](https://ldap.com/the-ldap-bind-operation/) operation with the LDAP server. Gloo Edge supports the following LDAP binding options: 
   - **User binding**: Gloo Edge extracts the username from the basic auth header, and substitutes the name with the `%s` placeholder in the `userDnTemplate` to build the DN for the `BIND` operation. Note that [special characters](https://ldapwiki.com/wiki/DN%20Escape%20Values) are removed from the username before performing the `BIND` operation to prevent injection attacks. Instead of user binding, you can use an LDAP service account to retrieve group membership information on behalf of the user.
   - **Service account binding**: Instead of giving each user access to the group membership information, you can use an LDAP service account to look up this information on behalf of the user. To authenticate with the LDAP server, you must store the LDAP service account credentials in a Kubernetes secret in your cluster. Then, you reference that secret in your `AuthConfig`. Note that you can only verify the user's group membership in the LDAP server with the service account. 
4. If the `BIND` operation fails when using user binding, the user is either unknown or their credentials are incorrect, and a `401` response code is returned. If the `BIND` operations fails for the service account, a `500` response code is returned. 
5. If the `BIND` operation is successful, issue a search operation using the `searchFilter` filter for the user entry (with a [`base` scope](https://ldapwiki.com/wiki/BaseObject)) and look 
   for an attribute with a name equal to `membershipAttributeName` on the user entry.
6. Check if one of the values for the attribute matches one of the `allowedGroups`; if so, allow the request, otherwise return a `403` response.

### Create an LDAP AuthConfig

The steps to create an LDAP AuthConfig vary depending on which LDAP binding option you choose. 

1. Create the LDAP AuthConfig. 

   {{< tabs >}}
   {{% tab name="User binding" %}}

   {{< highlight shell "hl_lines=10-13" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: ldap
  namespace: gloo-system
spec:
  configs:
  - ldap:
      address: "ldap://ldap.default.svc.cluster.local:389" # Substitute your namespace for `default` here
      userDnTemplate: "uid=%s,ou=people,dc=solo,dc=io"
      allowedGroups:
      - "cn=managers,ou=groups,dc=solo,dc=io"
      searchFilter: "(objectClass=*)"
EOF
   {{< /highlight >}}

   In this AuthConfig you can find the following settings: 
   - The configuration points to the Kubernetes DNS name and port of the LDAP service `ldap.default.svc.cluster.local:389` that you deployed earlier. 
   - Gloo Edge looks for user entries with DNs in the format `uid=<USERNAME_FROM_HEADER>,ou=people,dc=solo,dc=io`. This is the format of the user entry DNs the LDAP server was bootstrapped with.
   - Only members of the `cn=managers,ou=groups,dc=solo,dc=io` group can access the upstream.

   {{% /tab %}}
   {{% tab name="Service account" %}}
   
   {{% notice warning %}}
   For simplicity reasons, the following example uses the `admin` account as the service account. This setup is **NOT** a recommended security practice. If you plan to use this setup in production, make sure to set up a service account in your LDAP server that has the required permissions to look up group membership information on behalf of a user. Note that you can verify only the user's group membership in the LDAP server when using service account binding. For all other LDAP queries, user binding is used by default. 
   {{% /notice %}}

   1. Create a secret to store the credentials of the service account. 
      ```sh
      glooctl create secret authcredentials --name ldapcredentials --username cn=admin,dc=solo,dc=io  --password solopwd
      ```
   
   2. Create the Gloo Edge AuthConfig and enable group membership checking for the service account by setting the `checksGroupsWithServiceAccount` option to true. In addition, you must reference the secret that stores the credentials of the service account in the `credentialsSecretRef` field.  
      ```yaml
      kubectl apply -f - <<EOF
      apiVersion: enterprise.gloo.solo.io/v1
      kind: AuthConfig
      metadata:
        name: ldap
        namespace: gloo-system
      spec:
        configs:
        - ldap:
            address: "ldap://ldap.default.svc.cluster.local:389" # Substitute the default namespace if the ldap server was deployed to a different namespace
            userDnTemplate: "uid=%s,ou=people,dc=solo,dc=io"
            allowedGroups:
            - "cn=managers,ou=groups,dc=solo,dc=io"
            searchFilter: "(objectClass=*)"
            groupLookupSettings:
              checkGroupsWithServiceAccount: true
              credentialsSecretRef:
                name: ldapcredentials
                namespace: gloo-system
      EOF
      ```
   
   {{% /tab %}}
   {{< /tabs >}}
   
2. Edit the Virtual Service and reference the LDAP AuthConfig that you created. This setup configures the Virtual Service to use the `ldap` AuthConfig in the `gloo-system` namespace when authenticating requests to `/echo`.
   {{< highlight yaml "hl_lines=22-26" >}}
   {{< readfile file="guides/security/auth/extauth/ldap/vs-auth-ldap.sh" >}}
   {{< /highlight >}}

   
## Step 4: Verify LDAP auth for your Virtual Service

1. Verify that the Virtual Service behaves as expected. Because the Virtual Service is now enabled for LDAP auth, you must provide the user that you want to authorize in the basic auth header of your request. Note that all credentials in this header must be base64-encoded. You can use the values from the following table to build your basic auth header.  

   | username | password | basic auth header                         | comments                                    |
   |----------|----------|-------------------------------------------|---------------------------------------------|
   | marco    | marcopwd | Authorization: Basic bWFyY286bWFyY29wd2Q= | Member of "developers" group                |
   | rick     | rickpwd  | Authorization: Basic cmljazpyaWNrcHdk     | Member of "developers" and "managers" group |
   | john     | doe      | Authorization: Basic am9objpkb2U=         | Unknown user                                |

2. Send a request to `/echo` without any request headers and verify that you get back a `401` response code.
   ```shell script
   curl -v "$(glooctl proxy url)"/echo 
   ```
   
   Example output: 
   {{< highlight bash "hl_lines=9" >}}
* Trying 192.168.99.100...
* TCP_NODELAY set
* Connected to 192.168.99.100 (192.168.99.100) port 31940 (#0)
> GET /echo HTTP/1.1
> Host: 192.168.99.100:31940
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 401 Unauthorized
< date: Tue, 10 Sep 2019 17:14:39 GMT
< server: envoy
< content-length: 0
<
* Connection #0 to host 192.168.99.100 left intact
   {{< /highlight >}}
   
3. Send another request to the `/echo` endpoint. This time, you use an unknown user in the basic auth header. Verify that you get back a `401` response code.
   ```shell script
   curl -v -H "Authorization: Basic am9objpkb2U=" "$(glooctl proxy url)"/echo
   ```
   
   Example output: 
   {{< highlight bash "hl_lines=10" >}}
*   Trying 192.168.99.100...
* TCP_NODELAY set
* Connected to 192.168.99.100 (192.168.99.100) port 31940 (#0)
> GET /echo HTTP/1.1
> Host: 192.168.99.100:31940
> User-Agent: curl/7.54.0
> Accept: */*
> Authorization: Basic am9objpkb2U=
>
< HTTP/1.1 401 Unauthorized
< date: Tue, 10 Sep 2019 17:25:21 GMT
< server: envoy
< content-length: 0
<
* Connection #0 to host 192.168.99.100 left intact
   {{< /highlight >}}
   
4. Send another request and try to authenticate a user that belongs to the `developers` group. Because your AuthConfig allows only members of the `manager` group to access the endpoint, you get back a `403` response code. 
   ```shell script
   curl -v -H "Authorization: Basic bWFyY286bWFyY29wd2Q=" "$(glooctl proxy url)"/echo
   ```
   
   Example output: 
   {{< highlight bash "hl_lines=10" >}}
*   Trying 192.168.99.100...
* TCP_NODELAY set
* Connected to 192.168.99.100 (192.168.99.100) port 31940 (#0)
> GET /echo HTTP/1.1
> Host: 192.168.99.100:31940
> User-Agent: curl/7.54.0
> Accept: */*
> Authorization: Basic bWFyY286bWFyY29wd2Q=
>
< HTTP/1.1 403 Forbidden
< date: Tue, 10 Sep 2019 17:29:12 GMT
< server: envoy
< content-length: 0
<
* Connection #0 to host 192.168.99.100 left intact
   {{< /highlight >}}

5. Send another request and try to authenticate a user that belongs to the `managers` group. Verify that your request now succeeds. 
   ```shell script
   curl -v -H "Authorization: Basic cmljazpyaWNrcHdk" "$(glooctl proxy url)"/echo
   ```
   
   Example output: 
   {{< highlight bash "hl_lines=10 19" >}}
*   Trying 192.168.99.100...
* TCP_NODELAY set
* Connected to 192.168.99.100 (192.168.99.100) port 31940 (#0)
> GET /echo HTTP/1.1
> Host: 192.168.99.100:31940
> User-Agent: curl/7.54.0
> Accept: */*
> Authorization: Basic cmljazpyaWNrcHdk
>
< HTTP/1.1 200 OK
< x-app-name: http-echo
< x-app-version: 0.2.3
< date: Tue, 10 Sep 2019 17:30:12 GMT
< content-length: 15
< content-type: text/plain; charset=utf-8
< x-envoy-upstream-service-time: 0
< server: envoy
<
'Hello World!'
* Connection #0 to host 192.168.99.100 left intact
   {{< /highlight >}}
  
  
{{% notice tip %}}
If you use service account binding and get back a `500` response code, make sure that the credentials in your Kubernetes secret are correct. 
{{% /notice %}}



### Summary 
In this tutorial, you learned how Gloo Edge can integrate with LDAP to authenticate incoming requests and authorize them based 
on the group memberships of the user that was provided in the request.

To clean up the resources we created, you can run the following commands:

```bash
glooctl uninstall
kubectl delete configmap ldap
kubectl delete deployment ldap http-echo
kubectl delete service ldap http-echo
```
