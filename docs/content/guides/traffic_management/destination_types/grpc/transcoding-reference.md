---
title: Transcoding reference
weight: 40
description: Find examples for how to annotate your proto files with HTTP rule mappings so that Gloo Edge can correctly transform incoming HTTP requests. 
---

Review examples for how to annotate your proto files to add HTTP mappings. These HTTP mappings are used by Gloo Edge to transcode HTTP/ JSON requests to gRPC requests so that they can be forwarded to your gRPC upstream. The examples in this doc are based on the Bookstore app that you deploy as part of the [Transcode HTTP requests to gRPC]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/grpc-transcoding/">}}) guide. 

{{% notice tip %}}
To find more examples for how to add HTTP mappings to proto files and their syntax, refer to [Transcoding HTTP/JSON to gRPC](https://cloud.google.com/endpoints/docs/grpc/transcoding) in the Google Cloud documentation. 
{{% /notice %}}

On this page: 
- [Map a `List` method](#list)
- [Map a `Get` method](#get)
- [Map a `Create` method](#create)
- [Map a `Update` method](#update)
- [Map a `Delete` method](#delete)

## Map a `List` method {#list}

The `List` method is typically used to retrieve or search for a list of resources. 

### HTTP mapping rules
* The `List` method must be mapped to an HTTP GET request.
* The resource that you want to retrieve must be provided as part of the URL path. 
* Fields that are not provided as part of the URL path automatically become query parameters. 
* No request body can be specified as part of the HTTP request. 
* Retrieved objects are returned as a list in the response body.


### Example for getting a list of all resources

```
rpc ListShelves(google.protobuf.Empty) returns (ListShelvesResponse) {
    option (google.api.http) = {
      get: "/shelves"
    };
  }
  
message ListShelvesResponse {
  // Shelves in the bookstore.
  repeated Shelf shelves = 1;
}
```

In this example: 
* The ListShelves gRPC method is mapped to an HTTP GET request.
* `/shelves` is the URL path template that the GET request uses to call the ListShelves method. In this case, you want to return a list of all shelf resources.
* `google.protobug.Empty` specifies that no input parameters must be provided when calling the ListShelves gRPC method.
* The ListShelvesResponse specifies that multiple shelf resources are returned as a list in the HTTP response body. 

The code example implements the following HTTP to gRPC transcoding. 

|HTTP|gRPC|
|--|--|
|`curl -X GET http://{$DOMAIN_NAME}/shelves`|`ListShelves()`|


## Map a `Get` method {#get}

The `Get` method is typically used to retrieve a specific resource. 


### HTTP mapping rules
* The `Get` method must be mapped to an HTTP GET request.
* The name of the resource that you want to retrieve should be provided as part of the URL path. 
* Any other remaining fields that the gRPC request needs are retrieved and mapped from URL query parameters. 
* No request body can be defined as part of the HTTP request. 
* The retrieved resource must be returned as part of the HTTP response body.

### Example without query parameters

```
rpc GetAuthor(GetAuthorRequest) returns (Author) {
    option (google.api.http) = {
      get: "/authors/{author}"
    };
  }
  
message GetAuthorRequest {
  // The ID of the author resource to retrieve.
  int64 author = 1;
}

message Author {
  // A unique author id.
  int64 id = 1;
  enum Gender {
    UNKNOWN = 0;
    MALE = 1;
    FEMALE = 2;
  };
  Gender gender = 2;
  string first_name = 3;
  string last_name = 4 [json_name = "lname"];
}
```

In this example: 
* The GetAuthor gRPC method is mapped to an HTTP GET request.
* `/authors/{author}` is the URL path for the HTTP request. The `{author}` portion of the URL path instructs Gloo Edge to take the value that is provided in `{author}` and put it in the `author` parameter of the GetAuthorRequest. Because no other fields are defined in the GetAuthorRequest, any query parameters that are provided in the HTTP request are not mapped. 
* If the author is found, the details of the author are returned as specified in `message Author`. For example, information such as the ID, gender, and first name is returned in the HTTP response body. 


The code example implements the following HTTP to gRPC transcoding. 

|HTTP|gRPC|
|--|--|
|`curl -X GET http://{$DOMAIN_NAME}/authors/1`|`GetAuthor(author: "1")`|


## Map a `Create` method {#create}

The `Create` method is typically used to create a new resource under a specified parent. The newly created resource is then returned to the client. 

### HTTP mapping rules
* The `Create` method must be mapped to an HTTP POST request.
* The request message should have a field that specifies the parent resource name under which you want to create the resource.
* The details for the resource that you want to create must be provided in the HTTP request body in the format `body: "<resource_field>"`. 
* Any other remaining fields in the gRPC request are mapped to URL query parameters. 
* The created resource must be returned as part of the HTTP response body.

### Example to create a resource 

```
rpc CreateShelf(CreateShelfRequest) returns (Shelf) {
    option (google.api.http) = {
      post: "/shelf"
      body: "shelf"
    };
  }
  
message CreateShelfRequest {
  // The shelf resource to create.
  Shelf shelf = 1;
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
* `/shelf` is the URL path for the HTTP request and represents the parent resource under which you want to create the new resource.
* `body: "shelf"` specifies that all parameters that represent a shelf are provided as part of the HTTP request body. 
* `message Shelf` specifies the fields that a client must provide to create a shelf resource. For example, in order to create the shelf, you must send a value for the `id` and the `theme` in the HTTP request body.
* After the shelf resource is created, the details of the shelf as defined in `message Shelf` are returned in the HTTP response body. 

The code example implements the following HTTP to gRPC transcoding.

|HTTP|gRPC|
|--|--|
|`curl -X POST http://{$DOMAIN_NAME}/shelf -d {"id":"1234","theme":"drama"}`|`CreateShelf(id: "1234" theme: "drama")`|


### Example to create a resource with HTTP PUT

```
rpc CreateBook(CreateBookRequest) returns (Book) {
    option (google.api.http) = {
      put: "/shelves/{shelf}/books"
      body: "book"
    };
  }
  
message CreateBookRequest {
  // The ID of the shelf on which to create a book.
  int64 shelf = 1;
  // A book resource to create on the shelf.
  Book book = 2;
}

message Book {
  // A unique book id.
  int64 id = 1;
  // An author of the book.
  string author = 2;
  // A book title.
  string title = 3;
  // Quotes from the book.
  repeated string quotes = 4;
}
```

In this example: 
* The CreateBook gRPC method is mapped to an HTTP PUT request.
* `/shelves/{shelf}/books` is the URL path for the HTTP request and represents the parent resource under which you want to create the new resource. `{shelf}` represents the ID of the shelf and is mapped to the `shelf` parameter of the CreateBookRequest. 
* `body: book` specifies that all fields that represent a book are mapped from the HTTP request body. The fields that must be provided in the request body are defined in `message Book`. 
* After the book is created, the details of the book are returned in the HTTP response body as defined in `message Book`.

The code example implements the following HTTP to gRPC transcoding.

|HTTP | gRPC|
|-----|-----|
|`curl -X PUT http://{$DOMAIN_NAME}/shelves/1/books -d {"id":"50","author":"12345", "title": "The long ride"}`| `CreateBook(shelf: "1" book: Book(id: "50" author: "1234" title: "The long ride"))`|

## Map an `Update` method {#update}

The `Update` method is typically used to update the properties for a specific resource. After the resource is updated, it is returned to the client. 

### HTTP mapping rules 
* The `Update` method must allow partial updates for a given resource and is mapped to the HTTP PATCH request. Other updates, such as moving a resource to another parent cannot be implemented with an `Update` method. Instead, a custom method must be used. 
* If a resource only allows a full resource update, the `Update` method must be mapped to an HTTP PUT request. 
* The resource that you want to update must be provided in the URL path. 
* The properties that you want to update on the resource must be provided in the HTTP request body. 
* All remaining request message fields that are not provided in the URL path are mapped to the URL query parameters. 
* After the update is processed, the updated resource is returned to the client in the HTTP response body. 

### Example for updating a resource

```
rpc UpdateBook(UpdateBookRequest) returns (Book) {
    option (google.api.http) = {
      patch: "/shelves/{shelf}/books/{book.id}"
      body: "book"
    };
  }

message UpdateBookRequest {
  // The ID of the shelf from which to retrieve a book.
  int64 shelf = 1;
  // A book resource to update on the shelf.
  Book book = 2;
}

message Book {
  // A unique book id.
  int64 id = 1;
  // An author of the book.
  string author = 2;
  // A book title.
  string title = 3;
  // Quotes from the book.
  repeated string quotes = 4;
}
```

In this example: 
* The UpdateBook gRPC method is mapped to an HTTP PATCH request because it allows partial updates.
* `/shelves/{shelf}/books/{book.id}` is the URL path for the request. `{shelf}` represents the ID of the shelf where the book is stored. `{book.id}` represents the ID of the book that you want to update.
* `body: book` specifies that all remaining request fields that are not provided by the URL path template are mapped from the HTTP request body. In this example, fields such as the author or title must be provided in the HTTP request body. 

The code example implements the following HTTP to gRPC transcoding.

|HTTP | gRPC|
|-----|-----|
|`curl -X PATCH http://{$DOMAIN_NAME}/shelves/1/books/2 -d {"id":"2","author":"57", "title": "The last ride"}`| `UpdateBook(shelf: "1" book: Book(id: "2" author: "57" title: "The last ride"))`|


## Map a `Delete` method {#delete}

The `Delete` method is used to delete a specific resource. 

### HTTP mapping rules

* The `Delete` method must be mapped to an HTTP DELETE request. 
* The resource to delete should be provided as part of the URL path. 
* All remaining parameters for the gRPC request should be mapped to URL query parameters. 
* No request body can be provided in the HTTP request. 
* The `Delete` method immediately removes the resource. 
* The `Delete` method should return an empty response (`google.protobuf.Empty`). 

### Example for deleting a resource

```
rpc DeleteBook(DeleteBookRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/shelves/{shelf}/books/{book}"
    };
  }
  
message DeleteBookRequest {
  // The ID of the shelf from which to delete a book.
  int64 shelf = 1;
  // The ID of the book to delete.
  int64 book = 2;
}
```

In this example: 
* The DeleteBook gRPC method is mapped to an HTTP DELETE request.
* `/shelves/{shelf}/books/{book}` is the URL path for the HTTP request. `{shelf}` represents the ID of the shelf from which to delete the book. `{book}` is the ID of the book that you want to delete from the shelf. Both values are mapped to the `shelf` and `book` parameters in the DeleteBookRequest. 
* `google.protobuf.Empty` specifies that an empty HTTP response body is returned to the client. 

The code example implements the following HTTP to gRPC transcoding.

|HTTP | gRPC|
|-----|-----|
|`curl -X DELETE http://{$DOMAIN_NAME}/shelves/1/books/2`| `DeleteBook(shelf: "1" book: "2")`|






