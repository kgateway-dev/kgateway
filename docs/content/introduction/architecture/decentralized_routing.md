---
title: "Decentralized Ownership"
weight: 40
---

## Decentralized API ownership

Gloo’s developer-centric workflows can benefit from two specific parts of the Gloo configuration API. The first is that service teams completely own the {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} configuration in a “build it you run it” manner. The second is a [delegation model]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/delegation/" %}}) that allows service teams to own certain aspects of the configuration while allowing a centralized API team to own some of the [security and operation aspects]({{% versioned_link_path fromRoot="/security/" %}}). Let’s take a look at the second approach as it’s most applicable to our users and the workflows they wish to adopt.

## Delegate to developers

Gloo’s [configuration API enables developers]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/delegation/" %}}) to focus on the concerns they most care about ([routing tables]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/" %}}), [re-writing URLs]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/routing_features/prefix_rewrite/" %}}), [transformation of headers/body]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/routing_features/transformations/" %}}), [traffic shadowing]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/routing_features/shadowing/" %}}), etc) and offload the responsibility of configuring security and operational aspects to another team more suited for that (for compliance, organizational or other reasons). Gloo’s {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} configuration can delegate to {{< protobuf name="gateway.solo.io.RouteTable" display="RouteTable">}} objects which each developer or service team can own and potentially re-use.

{{<mermaid align="left">}}

graph LR;
    vs[Virtual Service <br> <br> <code>*.petclinic.com</code>] -->|delegate <code>/api</code> prefix | rt1(Route Table <br> <br> <code>/api/pets</code> <br> <code>/api/vets</code>)

    vs -->|delegate <code>/site</code> prefix | rt2(Route Table <br> <br> <code>/site/login</code> <br> <code>/site/logout</code>)

    style vs fill:#0DFF00,stroke:#233,stroke-width:4px

{{< /mermaid >}}

## Decentralize to go faster

With this delegation model, organizations can go faster without having to file tickets, wait for centralized teams, and give up control over how to manage their APIs. Platform teams can own and manage the parts of the API infrastructure that the organization demands. Developers can own the parts that are most relevant to them without the overhead and synchronization of a centralized team.

Organizational workflow, GitOps, simplicity, and self-service are a primary concern for cloud-native infrastructure, and these concerns are part of the foundation of the Gloo open-source API gateway project. We encourage you to take a look at Gloo or reach out about Gloo Enterprise and join the growing open-source community.
