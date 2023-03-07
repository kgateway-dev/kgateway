---
title: Reference
weight: 60
description: 
---

## Get a list of resources (HTTP GET to gRPC List) 

Map an HTTP GET method to a gRPC `List` method to retrieve a list of objects. 

### Code example

```shell
rpc ListShelves(google.protobuf.Empty) returns (ListShelvesResponse) {
    option (google.api.http) = {
      get: "/shelves"
    };
  }
```

In this example: 
* The ListShelves gRPC method is mapped to an HTTP GET request.
* `/shelves` is the URL path template that the GET request uses to call the `ListShelves` method. In this case, you want to return a list of all shelf resources.

### HTTP to gRPC mapping

The code example implements the following HTTP to gRPC transcoding. 

|HTTP|gRPC|
|--|--|
|`curl -X GET http://{$DOMAIN_NAME}/shelves`|`ListShelves()`|


## Get a specific resource (gRPC `Create` to HTTP POST)


### Code example

```
rpc GetAuthor(GetAuthorRequest) returns (Author) {
    option (google.api.http) = {
      get: "/authors/{author}"
    };
  }
```

In this example: 
* The GetAuthor gRPC method is mapped to an HTTP GET request.
* `/authors/{author}` is the URL path for the request. The `{author}` portion of the URL path instructs Gloo Edge to take the value that is provided in `{author}` and put it in the GetAuthor request parameter.

### HTTP to gRPC mapping

The code example implements the following HTTP to gRPC transcoding. 

|HTTP|gRPC|
|--|--|
|`curl -X GET http://{$DOMAIN_NAME}/authors/lee`|`GetAuthor(lee)`|





Map an HTTP POST method to a gRPC `Create` method to create an object. The details of the object are provided in the body of the HTTP request. 

### Code example

In the following example, you want to create a new shelf object. 

```
rpc CreateShelf(CreateShelfRequest) returns (Shelf) {
    option (google.api.http) = {
      post: "/shelf"
      body: "shelf"
    };
  }
```

* 


### HTTP to gRPC mapping

|HTTP|gRPC|
|--|--|
|`POST /shelf -d {"shelf-data"}`|`CreateShelf(shelf-data)`|



