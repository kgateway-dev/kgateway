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
* `/authors/{author}` is the URL path for the request. The `{author}` portion of the URL path instructs Gloo Edge to take the value that is provided in `{author}` and put it in the GetAuthorRequest parameter.

If a client sends a GET request to the http://mydomain/authors/lee URL, Gloo Edge creates a GetAuthorRequest with an author value of `lee`, and then uses this request to call the gRPC method GetAuthor(). The gRPC backend then returns the requested author with the name `lee`. Gloo Edge automatically converts this result into JSON format and returns it to the client.  

You can also add multiple request parameters that a client must specify as part of the URL. In the following example, a client must provide both the author and the author's book that they want to retrieve. 

```
rpc ListBooks(ListBooksRequest) returns (stream Book) {
    option (google.api.http) = {
      get: "/shelves/{shelf}/books/{book}"
    };
  }
```

### HTTP to gRPC mapping

|HTTP|gRPC|
|--|--|
|`curl -X GET http://{$DOMAIN_NAME}/authors/lee`|`GetAuthor(author: "lee")`|


## Create a resource (HTTP POST to gRPC Create)

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
  
message Shelf {
  // A unique shelf id.
  int64 id = 1;
  // A theme of the shelf (fiction, poetry, etc).
  string theme = 2;
}
```

In this example
* The CreateShelf gRPC method is mapped to an HTTP POST request.
* `/shelf` is the URL path for the request. 
* `body: "shelf"` specifies the resource that you want to add in JSON format. The details of the shelf resource are defined in the `message Shef` section. 
* To create a shelf object, you must provide a JSON object as part of the request body that contains an ID and the theme of the shelf resource that you want to add. 


### HTTP to gRPC mapping

|HTTP|gRPC|
|--|--|
|`curl -X POST http://{$DOMAIN_NAME}/shelf -d {"id:1234","theme":"drama"}`|`CreateShelf(id: "1234" theme: "drama")`|



