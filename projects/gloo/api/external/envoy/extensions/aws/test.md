```mermaid
graph TD;
subgraph Data Plane
eg
egee
end
subgraph Control Plane
gloo
sp
end
  eg[envoy-gloo] --> egee[envoy-gloo-ee];
  eg --> gloo;
  egee --> sp[solo-projects];
  gloo --> sp
```