# MCP Tests details

For the dynamic routing we use two MCP servers one "user" and other "admin". 

Probably it can be simplified. For now we use two different docker images.

- User container uses `ghcr.io/peterj/mcp-website-fetcher:main` as a source and is copied to the CI registry on GitHub as `ghcr.io/kgateway-dev/mcp-website-fetcher:0.0.1`
- Admin container is built using upstream and placed `ghcr.io/kgateway-dev/mcp-admin-server:0.0.1` it can be rebuilt using Dockerfile in this directory. Use `docker build -t ghcr.io/kgateway-dev/mcp-admin-server:<version> .` and then push it to GH with `docker push ghcr.io/kgateway-dev/mcp-admin-server:<version>` command