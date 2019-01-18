## External API Routing

In this tutorial, we'll take a look at routing to services that live outside of your deployment platform and that have not been configured for automatic discovery. You can consider that these could be existing monoliths or static endpoints that do not lend themselves easily to be discovered. Gloo's power comes from it's ability to live in unpredictable and dynamic environments just fine, but for those use cases where we need to explicitly add upstreams, we can do that following these steps.


### What you'll need

You'll need to have Gloo installed on Kubernetes and have access to that Kubernetes cluster. Please refer to the [Gloo installation](../../installation/kubernetes.md) for guidance on installing Gloo into Kubernetes. 

You'll also need access from the Kubernetes cluster to an external API. You can use whichever external API you wish; we'll use an API called [JSONPlaceholder](https://jsonplaceholder.typicode.com) which simulates a REST API for basic testing. 

To route to an external API, we need to first create a Gloo [upstream](../../v1/upstream.proto.sk.md). A quick recap will show that a Gloo upstream is a network entry (think _host:port_) in the Gloo service catalog (or ["cluster" as Envoy proxy calls it](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/cluster_manager)). We'll use [glooctl create upstream](../../cli.md#create-upstreams) command to do this:

        glooctl create upstream static jsonplaceholder-80 --static-hosts jsonplaceholder.typicode.com:80
               
        +--------------------+--------+---------+---------------------------------+
        |      UPSTREAM      |  TYPE  | STATUS  |             DETAILS             |
        +--------------------+--------+---------+---------------------------------+
        | jsonplaceholder-80 | Static | Pending | hosts:                          |
        |                    |        |         | -                               |
        |                    |        |         | jsonplaceholder.typicode.com:80 |
        |                    |        |         |                                 |
        +--------------------+--------+---------+---------------------------------+
        
        
In this case, we created a `static` upstream which means this is not something Gloo dynamically discovered on its own using its powerful upstream and function discovery mechanisms but rather that we added it explicitly. Feel free to explore the other `glooctl create upstream` options to creat additional upstream entries.         

Gloo should now know about our `jsonplaceholder` upstream. To verify run `glooctl get upstream -n default` and notice the `Status` column:

        glooctl get upstream -n default
        
        
        glooctl get upstream -n default
        +--------------------+--------+----------+---------------------------------+
        |      UPSTREAM      |  TYPE  |  STATUS  |             DETAILS             |
        +--------------------+--------+----------+---------------------------------+
        | jsonplaceholder-80 | Static | Accepted | hosts:                          |
        |                    |        |          | -                               |
        |                    |        |          | jsonplaceholder.typicode.com:80 |
        |                    |        |          |                                 |
        +--------------------+--------+----------+---------------------------------+
        
Note that we explicitly specified the namespace otherwise `glooctl` would default to the `gloo-system` namespace.

Let's add a route like we did in the [basic routing tutorial](./basic_routing.md) to route incoming traffic from `/api/posts` to our new jsonplaceholder upstream:

        
        glooctl add route \
           --dest-name jsonplaceholder-80 \
           --dest-namespace default \
           --path-exact /api/posts \
           --prefix-rewrite /posts
        
        creating virtualservice default with default domain *
        +-----------------+---------+------+---------+---------+--------------------------------+
        | VIRTUAL SERVICE | DOMAINS | SSL  | STATUS  | PLUGINS |             ROUTES             |
        +-----------------+---------+------+---------+---------+--------------------------------+
        | default         | *       | none | Pending |         | /api/posts ->                  |
        |                 |         |      |         |         | jsonplaceholder-80             |
        +-----------------+---------+------+---------+---------+--------------------------------+

Let's try calling our new API:

        export GATEWAY_URL=$(glooctl gateway url)
        curl ${GATEWAY_URL}/api/posts      
        
Now we should see output similar to:

        ...
        
        
          {                                                                                                                                                                                 
            "userId": 1,                                     
            "id": 1,                                                                                                                                                                
            "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",                                                                                                                               
            "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"                                          
          },                                                                                                                                                                                                                      
          {                                                                                                                                       
            "userId": 1,                                               
            "id": 2,                                                                                                                                            
            "title": "qui est esse",                                                                                                                                                                                             
            "body": "est rerum tempore vitae\nsequi sint nihil reprehenderit dolor beatae ea dolores neque\nfugiat blanditiis voluptate porro vel nihil molestiae ut reiciendis\nqui aperiam non debitis possimus qui neque nisi $
        ulla"                                                                                                                                                                                                                     
          },
          {
            "userId": 1,
            "id": 3,
            "title": "ea molestias quasi exercitationem repellat qui ipsa sit aut",
            "body": "et iusto sed quo iure\nvoluptatem occaecati omnis eligendi aut ad\nvoluptatem doloribus vel accusantium quis pariatur\nmolestiae porro eius odio et labore et velit aut"
          },
          {
            "userId": 1,
            "id": 4,
            "title": "eum et est occaecati",
            "body": "ullam et saepe reiciendis voluptatem adipisci\nsit amet autem assumenda provident rerum culpa\nquis hic commodi nesciunt rem tenetur doloremque ipsam iure\nquis sunt voluptatem rerum illo velit"
          },
          {
            "userId": 1,
            "id": 5,
            "title": "nesciunt quas odio",
            "body": "repudiandae veniam quaerat sunt sed\nalias aut fugiat sit autem sed est\nvoluptatem omnis possimus esse voluptatibus quis\nest aut tenetur dolor neque"
          },
          {
            "userId": 1,
            "id": 6,
            "title": "dolorem eum magni eos aperiam quia",
            "body": "ut aspernatur corporis harum nihil quis provident sequi\nmollitia nobis aliquid molestiae\nperspiciatis et ea nemo ab reprehenderit accusantium quas\nvoluptate dolores velit et doloremque molestiae"
          },
          
          ...
                  