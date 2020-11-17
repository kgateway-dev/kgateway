---
title: "Gloo Portal"
weight: 70
description: Publish interactive documentation for your APIs
---

In this guide we will see how to publish interactive documentation for your APIs and expose it to users via the 
Gloo Edge Enterprise Portal.

## Important Update
We recently released an updated version of the Gloo Portal. The new Gloo Portal adds a number of new features, 
including the ability to automatically generate routing configuration for your Gloo Edge installation based on your service 
specifications.

The new Gloo Portal is a standalone application that can be installed on top of **Gloo Edge Enterprise** starting with 
release v1.5.0-beta10. For more information, check out the 
[new Gloo Portal documentation](https://docs.solo.io/gloo-portal/latest).

## Initial setup
{{% notice warning %}}
The Gloo Portal feature described on this page was introduced with **Gloo Edge Enterprise**, release v1.3.0 and 
deprecated with **Gloo Edge Enterprise**, release v1.5.0-beta10. 
If you are using an earlier version, this feature will not be available. If you are running this (or a later) version of 
Gloo Edge Enterprise, use [these installation instructions](https://docs.solo.io/gloo-portal/latest/setup/gloo).
{{% /notice %}}

Before we can start configuring our portal, we need to enable the Gloo Portal feature in Gloo Edge and configure our 
cluster with an example application and the corresponding routing configuration.

#### Enabling the Gloo Portal feature in Gloo Edge
The Gloo Edge Portal can be installed as part of Gloo Edge Enterprise by providing an additional `devPortal=true` Helm 
value during your installation or upgrade process. Please refer to the Gloo Edge Enterprise [installation guide](https://docs.solo.io/gloo-edge/latest/installation/enterprise/) 
for more details on the various installation options.

You can install Gloo Edge Enterprise with the Gloo Portal either via `helm` or via `gloooctl`:

{{< tabs >}}
{{% tab name="helm" %}}
```shell script
helm install glooe glooe/gloo-ee --namespace gloo-system \
  --set-string license_key=YOUR_LICENSE_KEY \
  --set devPortal.enabled=true
```
{{% /tab %}}
{{% tab name="glooctl" %}}

First we need to create a values file:

```shell script
cat << EOF > values.yaml
devPortal:
  enabled: true
EOF
```

Then we can install with the above values:

```shell script
glooctl install gateway enterprise --license-key YOUR_LICENSE_KEY --values values.yaml
```
{{% /tab %}}
{{< /tabs >}}

If the installation was successful you should see the following when running `kubectl get pods -n gloo-system`:

```
api-server-7bff99588f-r6bjl                            3/3     Running   0          83s
dev-portal-6f5f6899cc-kqtwt                            1/1     Running   0          83s
discovery-6fd479f868-9dxhk                             1/1     Running   0          83s
extauth-6d9b44ccbd-qrjc6                               1/1     Running   0          83s
gateway-5c4c88d85c-5czml                               1/1     Running   0          83s
gateway-proxy-d8758d4d4-l49w9                          1/1     Running   0          83s
gloo-56c69d7d7f-qxvkd                                  1/1     Running   0          83s
glooe-grafana-649b9fc9b4-2rqwd                         1/1     Running   0          83s
glooe-prometheus-kube-state-metrics-866856df4d-rsmz2   1/1     Running   0          83s
glooe-prometheus-server-7f85c5778d-vm88v               2/2     Running   0          83s
observability-df76bd88c-8tcnn                          1/1     Running   0          83s
rate-limit-68958d7bb-h4zmm                             1/1     Running   2          83s
redis-9d9b9955f-bh68s                                  1/1     Running   0          83s
```

Notice the `dev-portal-6f5f6899cc-kqtwt` pod.

#### Deploy a sample application
Let's deploy an application that will represent the service that we want to publish an API for:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: petstore
  name: petstore
  namespace: gloo-system
spec:
  selector:
    matchLabels:
      app: petstore
  replicas: 1
  template:
    metadata:
      labels:
        app: petstore
    spec:
      containers:
      - image: soloio/petstore-example:latest
        name: petstore
        ports:
        - containerPort: 8080
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: petstore
  namespace: gloo-system
  labels:
    sevice: petstore
spec:
  ports:
  - port: 8080
    protocol: TCP
  selector:
    app: petstore
```

Let's also create a Gloo Edge virtual service that will route all requests for paths starting with `api` to this service:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
  namespace: gloo-system
spec:
  virtualHost:
    options:
      cors:
        allowOrigin:
        - "http://localhost:1234"
    domains:
    - "localhost:8080"
    routes:
    - matchers:
      - prefix: /api
      routeAction:
        single:
          kube:
            port: 8080
            ref:
              name: petstore
              namespace: gloo-system
```

We will come back and explain the reason behind the domain and the CORS configuration [later in this guide](#note-on-cors).

Let's verify that everything works as expected:

``` 
curl -H "host: localhost:8080" $(glooctl proxy url)/api/pets -v
```

The above request should result in the following response:
``` 
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## Access the Gloo Portal administrator UI
Gloo Portals can be managed through a the Gloo Edge Enterprise UI. To access the UI run:

```shell 
kubectl port-forward -n gloo-system deployment/api-server 8081:8080
```

If you open your browser and navigate to `localhost:8081`, you should see the Gloo Edge Enterprise UI landing page:
![Gloo Edge Enterprise UI]({{% versioned_link_path fromRoot="/guides/dev_portal/img/ui-landing-page.png" %}})

If the Gloo Portal was successfully installed and your license key is valid you should see the "Gloo Portal" link 
in the top right corner of the screen. If you click on it, you will see the Gloo Portal overview page. This page 
presents a summary of the main Gloo Portal resources (nothing interesting at this point, since we did not create 
any resources yet).
![Empty landing page]({{% versioned_link_path fromRoot="/guides/dev_portal/img/dev-portal-empty-landing.png" %}})

## Create a portal
Let's start by creating a portal. From the overview page click on the "View Portals" link and then on the 
"Create a Portal" button. This will display the portal creation wizard.

##### General portal info
The first step prompts you to define the basic attributes of your portal:

1. `Name`: this will be title of the portal
2. `Description`: short description text that will be displayed close to the title on the portal
3. `Portal Domain(s)`: these are the domains that are associated with the portal. The Gloo Portal web server will 
decide which portal to server to a user based on the host header contained in the user request. For example, if you are 
planning on serving your portal from `my-portal.example.org`, you will need to include this host name in the portal 
domains. When a user navigates to `my-portal.example.org`, the Gloo Portal web server will verify if any portal is 
associated with that host and, if so, display it.

{{% notice note %}}
Portal domains must not overlap. In case multiple portals specify overlapping domains, the Gloo Portal controller 
will reject the most recently updated portal resources, i.e. it will accept the portal that first defined the 
overlapping domain.
{{% /notice %}}

We will set the domain to `localhost:1234`, as in this guide we will be port-forwarding the portal server to our local 
machine.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-wizard-1.png" %}})

##### Portal banner
The next step will prompt us to upload a "hero" image. This is a large banner that will serve as a background for our 
portal. Static portal assets like images are stored in config maps, so the maximum size is determined the 
[`--max-request-bytes`](https://github.com/etcd-io/etcd/blob/master/Documentation/dev-guide/limit.md) `etcd` option for 
your cluster. The default for `etcd` is 1.5MiB, but this varies depending on your Kubernetes configuration.

##### Branding logos
The next step allow you to upload a logo and favicon for your portal.

##### Portal access
The next three steps allow you to determine which APIs will be published to the portal and which users ans groups should 
access to the portal. As we did not create any of these entities yet, we will just skip these steps and submit the form 
to create the portal.

##### Result
You should see the details of the portal we just created:
![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-details-1.png" %}})

### Adding static pages to the portal
A common feature of Gloo Portals is to allow the administrator to add custom pages to the web application. We can 
do that by visiting the portal details page on the Gloo Edge Enterprise UI again, selecting the "Pages" tab in the lower 
part of the screen and clicking the "Add a Page" button. 
The resulting form prompts us for the basic properties of a static portal page.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/static-page-1.png" %}})

1. `Page Name`: the display name for this page
2. `Page URL`: the URL at which this  page will be available
3. `Page Description`: description for the page
4. `Navigation Link Name`: this determines the name of the link that will be displayed on the portal navigation bar
5. `Display on Home Page`: check this box if you want to display a tile with the name and description of the page on 
the portal home page. Clicking the tile will open the page.

After submitting the form you should see that the static page has been added to the portal.
Let's click on the edit button in the "Actions" column of the "Pages" table. This will display an editor where you can 
define the content of the page using markdown.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/static-page-2.png" %}})

You can preview the page by clicking the "Preview Changes" button.
![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/static-page-3.png" %}})

When you are done, click "Publish changes" to publish the static page..

#### Test the portal
Now let's see how the portal looks like! Portals are served by the web server that listens on port `8080` of the 
`dev-portal` deployment, so let's first port-forward the web server to our local machine:

```shell 
kubectl port-forward -n gloo-system deploy/dev-portal 1234:8080
```

We are forwarding to port `1234` on localhost as this is the domain we configured on the portal earlier. If you now open 
your browser and navigate to `localhost:1234` you should see the portal home page.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-home-1.png" %}})

Note the portal branding and the static markdown page we configured earlier.

## Give users access to the portals
The portal home page and the static pages it defines are publicly accessible, but to be able to access the APIs 
published in the portal users need to log in.
Let's create a user, assign them to a group, and give the group access to the portal we just created. Users can also 
be given direct access to resources (portals, APIs)

#### Create a group
From the overview screen, click on "View Users & Groups" link and then on the "Create a Group" button. 
This will display the group creation wizard.

1. The first step will prompt you for a name and an optional description for the group;
2. In the next step we could add users to the group (but we don't have any yet);
3. In the next step we can select the APIs that members of the group will have access to; we don't have any APIs yet, so let's skip this for now;
4. In the final step we can decide which portals the members of the group will have access to; let's select the portal we just created and submit the form.

If everything went well you should see the details of the group.
![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/group-1.png" %}})

#### Create a user
Now let's add a user to the group by clicking on the "Create a User" button. This will display the user creation wizard.

1. In the first step we have to add the usual info for a user:
    - `Name`: this is the username, it is required and must be unique within the cluster (it can be an email address)
    - `Email`: the user email address (optional)
    - `Password`: the initial user password; this is required. When the user logs in to a portal for the first time, they 
     will be prompted to update their password. User credential distribution is currently up to the Gloo Edge administrator, 
     but we plan on adding a standard email verification flow soon;
2. In the next step we can select the APIs that this user has direct access to (i.e. not through a group); 
we don't have any APIs yet, so let's skip this for now;
3. The final step allows us the give the user direct access to a portal; we don't need this as we want the user to 
have access through the group.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/user-1.png" %}})

#### Log into the portal
Now that we have created a user, let's go back to our portal at `localhost:1234` and click the login button in the top 
right corner of the screen. Input the username and password for the user we just created and you will be prompted to 
update your password. Choose a new password, submit the form and you will be logged into the portal.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/password-change.png" %}})

If you click on the `APIs` tab in the navigation bar you will see that it no longer asks you to log in.
Since we did not publish an API there is not much else we can do with the portal at this point, so let's go ahead and 
publish our first API!

## Publish an API
In this section we will see how to publish interactive OpenAPI documentation to your portal.

#### Create an OpenAPI document for our service
Before we can publish an API, we need to create an OpenAPI document that describes the service it represents. 
the document does not need to match the entirety of the endpoints exposes by your service. You can choose to 
expose only a subset of the endpoints, for example if you want to expose just a part of a larger monolithic application. 
It is up to the user to verify that the document matches the service it describes; the Gloo Portal will not attempt 
to validate the document against the service.

{{% expand "Here is the full OpenAPI document representing our example application (click to expand)" %}}
```json
{
  "swagger": "2.0",
  "info": {
    "version": "1.0.0",
    "title": "Swagger Petstore",
    "description": "A sample API that uses a petstore as an example to demonstrate features in the swagger-2.0 specification",
    "termsOfService": "http://helloreverb.com/terms/",
    "contact": {
      "name": "Wordnik API Team"
    },
    "license": {
      "name": "MIT"
    }
  },
  "host": "localhost:8080",
  "basePath": "/api",
  "schemes": [
    "http"
  ],
  "consumes": [
    "application/json"
  ],
  "produces": [
    "application/json"
  ],
  "paths": {
    "/pets": {
      "get": {
        "description": "Returns all pets from the system that the user has access to",
        "operationId": "findPets",
        "produces": [
          "application/json",
          "application/xml",
          "text/xml",
          "text/html"
        ],
        "parameters": [
          {
            "name": "tags",
            "in": "query",
            "description": "tags to filter by",
            "required": false,
            "type": "array",
            "items": {
              "type": "string"
            },
            "collectionFormat": "csv"
          },
          {
            "name": "limit",
            "in": "query",
            "description": "maximum number of results to return",
            "required": false,
            "type": "integer",
            "format": "int32"
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "type": "array",
              "items": {
                "$ref": "#/definitions/pet"
              }
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      },
      "post": {
        "description": "Creates a new pet in the store.  Duplicates are allowed",
        "operationId": "addPet",
        "produces": [
          "application/json"
        ],
        "parameters": [
          {
            "name": "pet",
            "in": "body",
            "description": "Pet to add to the store",
            "required": true,
            "schema": {
              "$ref": "#/definitions/petInput"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "$ref": "#/definitions/pet"
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      }
    },
    "/pets/{id}": {
      "get": {
        "description": "Returns a user based on a single ID, if the user does not have access to the pet",
        "operationId": "findPetById",
        "produces": [
          "application/json",
          "application/xml",
          "text/xml",
          "text/html"
        ],
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "description": "ID of pet to fetch",
            "required": true,
            "type": "integer",
            "format": "int64"
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "$ref": "#/definitions/pet"
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      },
      "delete": {
        "description": "deletes a single pet based on the ID supplied",
        "operationId": "deletePet",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "description": "ID of pet to delete",
            "required": true,
            "type": "integer",
            "format": "int64"
          }
        ],
        "responses": {
          "204": {
            "description": "pet deleted"
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      }
    }
  },
  "definitions": {
    "pet": {
      "required": [
        "id",
        "name"
      ],
      "properties": {
        "id": {
          "type": "integer",
          "format": "int64"
        },
        "name": {
          "type": "string"
        },
        "tag": {
          "type": "string"
        }
      }
    },
    "petInput": {
      "allOf": [
        {
          "$ref": "#/definitions/pet"
        },
        {
          "required": [
            "name"
          ],
          "properties": {
            "id": {
              "type": "integer",
              "format": "int64"
            }
          }
        }
      ]
    },
    "errorModel": {
      "required": [
        "code",
        "message"
      ],
      "properties": {
        "code": {
          "type": "integer",
          "format": "int32"
        },
        "message": {
          "type": "string"
        }
      }
    }
  }
}
```
{{% /expand %}}

Please note the following two attributes in out document:

- the `host` attribute, which we set to `localhost:8080`
- the `basePath` which we set to `/api`

The interactive documentation for API that will be published in the portal will allow users to test the endpoints of the 
API. These above attributes will determine the base of the address that the requests will be sent to, in this case:

`localhost:8080/api`

Please save the above document to your local file system for the next step 
(you can download it [here]({{% versioned_link_path fromRoot="/guides/dev_portal/specs/petstore.json" %}})).

#### Create an API
Let's go back to the Gloo Edge Enterprise UI, and navigate to the "APIs" section of the Gloo Portal screen. Click on the 
"Create an API" button to display the API creation wizard.

1. The first step requires you to upload an OpenAPI document that represent your API. You can either provide a URL or 
upload the file from you local file system. Here you can
2. Next you can upload an image for your API. The image will be displayed next to the API in the portal.
3. In the following screen select our portal.
4. Skip the user step (as the user will have access through the group)
5. Add the group to give it access to this API and submit the form.

You should now see the details of the API we just created. The Gloo Portal server will parse the OpenAPI document 
and display some of the properties of the document, for example the display name and the description. In the lower part 
of the screen you can see which groups and users are allowed to see this API (if it is published to a portal they have 
access to).

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/api-1.png" %}})

#### View and test the API
Let's go back to our portal at `localhost:1234` and click the "API" button in the navigation bar. You should now see 
an entry for our newly published API.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-1.png" %}})

If you click on the document you can browse through all of the info that we included in our OpenAPI document above. 

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-2.png" %}})

Before we test our interactive document, we need to port forward the Gloo Edge gateway proxy (which the document expects to 
be listening at `localhost:8080`):

```
kubectl port-forward -n gloo-system deploy/gateway-proxy 8080
```

Now let's try and query the "GET /pets" endpoint:

1. expand the endpoint entry
2. click the "Try it out" button
3. click "Execute"

You should see the response from the server in the "Server response" section.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-3-no-auth-ok.png" %}})

## Secure an API
We were able to query the published API without providing any credentials, but in a real-world scenario access to the 
API will most likely need to be secured. The Gloo Edge Enterprise Gloo Portal currently supports self-service for APIs 
that are secured using API keys. It does so by leveraging the Gloo Edge Enterprise 
[API keys]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/apikey_auth" %}}) authentication feature. 
In the following sections we will see how to configure Gloo Edge to allow users to generate API keys to send authenticated 
requests via the interactive API document.

#### Create a key scope
An API key scope is a way of grouping APIs that share a common API key configuration within the context of a portal. 
The portal server uses the key scope information to generate API key secrets. When a user requests an API key for a 
particular key scope, the server will generate an API key secret that can be consumed by Gloo Edge. 
This will become easier to understand after seeing a concrete example. 

Let's go back to the Gloo Edge Enterprise UI, and navigate to the "API Key Scopes" section of the Gloo Portal screen. 
Click the "Create a Scope" button to open the API key scope creation wizard. 

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/key-scope-1.png" %}})

You will need to provide:

1. A name for the key scope
2. The portal the key scope belongs to
3  The API(s) that share this key scope

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/key-scope-2.png" %}})

Each secret generated for a given key scope will contain a label with the following format in its metadata:

`portals.devportal.solo.io/<PORTA_NAMESPACE>.<PORTAL_NAME>.<KEY_SCOPE_NAME>: "true"`

For example, given the resources we created up to this point, the secrets created for the above key scope will contain 
the following label:

`portals.devportal.solo.io/gloo-system.pet-store.pet-key-scope: "true"`

#### Add API key auth to the API
With the above information, we can go ahead and update our virtual service. First, let's create an API key `AuthConfig`:

```
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: apikey-auth
  namespace: gloo-system
spec:
  configs:
  - api_key_auth:
      label_selector:
        portals.devportal.solo.io/gloo-system.pet-store.pet-key-scope: "true"
EOF
```

This will allow any requests that provide an API key that is contained in a kubernetes secret that matches the given label.

Let's update our virtual service to use this `AuthConfig`:

{{< highlight yaml "hl_lines=12-17" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
  namespace: gloo-system
spec:
  virtualHost:
    options:
      cors:
        allowOrigin:
        - "http://localhost:1234"
        allowHeaders:
        - "api-key"
      extauth:
        configRef:
          name: apikey-auth
          namespace: gloo-system
    domains:
    - "localhost:8080"
    routes:
    - matchers:
      - prefix: /api
      routeAction:
        single:
          kube:
            port: 8080
            ref:
              name: petstore
              namespace: gloo-system
{{< /highlight >}} 

Note that we also added `api-key` to the headers allowed by our CORS configuration. `api-key` is the name of the header 
that Gloo Edge inspects for an API key. The interactive documentation will send requests with that header to the service.

##### Note on CORS
We need the CORS configuration because the portal app is served from `localhost:1234`, but the interactive API document 
sends requests to `localhost:8080`. Without it, the request violated the same-origin security policy. 
The CORS configuration allows your browser and Gloo Edge to validate the cross-origin request.

#### Update our OpenAPI document
Next we need to update our API specification in order to account for the changes we just made, so let's go back to the 
details page for our API in the Gloo Edge Enterprise UI. The UI provides an OpenAPI editor that we can use to update our 
document. You can open it by clicking the "Open editor" button in the lower left corner. 

We need to add these attributes too the root object:

```json
{
  "securityDefinitions": {
    "petStoreApiKey": {
      "type": "apiKey",
      "name": "api-key",
      "in": "header"
    }
  },
  "security": [
    {
      "petStoreApiKey": []
    }
  ]
}
```

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/api-2-editor.png" %}})

The `securityDefinitions` object is a declaration of the security schemes available to be used in the specification. 
This does not enforce the security schemes on the operations and only serves to provide the relevant details for each scheme.
Here we are defining an API key authentication scheme that expects API keys to be included in a header named `api-key`.

{{% notice note %}}
It is important that the header name is `api-key`, otherwise Gloo Edge will not be able to authenticate the requests sent from the document.
{{% /notice %}}

The `security` attribute lists the required security schemes to execute an operation. Since we define it at the root of 
the document, it will apply to all operations.

Please check out [this page](https://swagger.io/resources/open-api/) for more info about the OpenAPI specifications.

After you have applied the changes, you can preview them by clicking "Update preview". If everything looks fine, 
save the changes by clicking "Publish Changes".

{{% expand "Click to see the complete updated document" %}}
```json
{
  "swagger": "2.0",
  "info": {
    "version": "1.0.0",
    "title": "Swagger Petstore",
    "description": "A sample API that uses a petstore as an example to demonstrate features in the swagger-2.0 specification",
    "termsOfService": "http://helloreverb.com/terms/",
    "contact": {
      "name": "Wordnik API Team"
    },
    "license": {
      "name": "MIT"
    }
  },
  "host": "localhost:8080",
  "basePath": "/api",
  "schemes": [
    "http"
  ],
  "consumes": [
    "application/json"
  ],
  "produces": [
    "application/json"
  ],
  "securityDefinitions": {
    "petStoreApiKey": {
      "type": "apiKey",
      "name": "api-key",
      "in": "header"
    }
  },
  "security": [
    {
      "petStoreApiKey": []
    }
  ],
  "paths": {
    "/pets": {
      "get": {
        "description": "Returns all pets from the system that the user has access to",
        "operationId": "findPets",
        "produces": [
          "application/json",
          "application/xml",
          "text/xml",
          "text/html"
        ],
        "parameters": [
          {
            "name": "tags",
            "in": "query",
            "description": "tags to filter by",
            "required": false,
            "type": "array",
            "items": {
              "type": "string"
            },
            "collectionFormat": "csv"
          },
          {
            "name": "limit",
            "in": "query",
            "description": "maximum number of results to return",
            "required": false,
            "type": "integer",
            "format": "int32"
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "type": "array",
              "items": {
                "$ref": "#/definitions/pet"
              }
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      },
      "post": {
        "description": "Creates a new pet in the store.  Duplicates are allowed",
        "operationId": "addPet",
        "produces": [
          "application/json"
        ],
        "parameters": [
          {
            "name": "pet",
            "in": "body",
            "description": "Pet to add to the store",
            "required": true,
            "schema": {
              "$ref": "#/definitions/petInput"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "$ref": "#/definitions/pet"
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      }
    },
    "/pets/{id}": {
      "get": {
        "description": "Returns a user based on a single ID, if the user does not have access to the pet",
        "operationId": "findPetById",
        "produces": [
          "application/json",
          "application/xml",
          "text/xml",
          "text/html"
        ],
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "description": "ID of pet to fetch",
            "required": true,
            "type": "integer",
            "format": "int64"
          }
        ],
        "responses": {
          "200": {
            "description": "pet response",
            "schema": {
              "$ref": "#/definitions/pet"
            }
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      },
      "delete": {
        "description": "deletes a single pet based on the ID supplied",
        "operationId": "deletePet",
        "parameters": [
          {
            "name": "id",
            "in": "path",
            "description": "ID of pet to delete",
            "required": true,
            "type": "integer",
            "format": "int64"
          }
        ],
        "responses": {
          "204": {
            "description": "pet deleted"
          },
          "default": {
            "description": "unexpected error",
            "schema": {
              "$ref": "#/definitions/errorModel"
            }
          }
        }
      }
    }
  },
  "definitions": {
    "pet": {
      "required": [
        "id",
        "name"
      ],
      "properties": {
        "id": {
          "type": "integer",
          "format": "int64"
        },
        "name": {
          "type": "string"
        },
        "tag": {
          "type": "string"
        }
      }
    },
    "petInput": {
      "allOf": [
        {
          "$ref": "#/definitions/pet"
        },
        {
          "required": [
            "name"
          ],
          "properties": {
            "id": {
              "type": "integer",
              "format": "int64"
            }
          }
        }
      ]
    },
    "errorModel": {
      "required": [
        "code",
        "message"
      ],
      "properties": {
        "code": {
          "type": "integer",
          "format": "int32"
        },
        "message": {
          "type": "string"
        }
      }
    }
  }
}
```
{{% /expand %}}

#### Test the updated doc
Now that everything is in place, let's go back to the portal and open the interactive API doc. If you try querying the 
same endpoint we tested earlier, you will now get a 401 Unauthorized response. This is Gloo Edge rejecting the request 
because it did not provide an API key.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-4-no-auth-ko.png" %}})

Now click on the user icon in the top right corner and select "API keys". This will open a page where the user can see 
the key scopes that are available for the APIs they have access to. 

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-key-scopes-1.png" %}})

Click on "generate an API key" and confirm. You should see an API key appear in the key scope. 

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-key-scopes-2.png" %}})

Click it to copy it to the clipboard and head back to the API document. 
Click the "Authorize" button (which is displayed now that we have added a `securityDefinition`), paste the API key into 
the text field and confirm.

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-5-auth-dialog.png" %}})

Now try the endpoint again and it should work!

![]({{% versioned_link_path fromRoot="/guides/dev_portal/img/portal-api-6-auth-ok.png" %}})