# UDPRoute

kgateway translates a UDPRoute into an Envoy `udp_proxy` listener filter. This page describes the
behavior kgateway commits to, so it does not depend on Envoy defaults. [GEP-2645](https://gateway-api.sigs.k8s.io/geps/gep-2645/)
leaves UDP session management to implementations and asks them to document it.

UDPRoute requires Gateway API v1.6.0 or later, where UDPRoute is served as `v1`. With older Gateway
API CRDs, kgateway does not reconcile UDPRoutes and logs a warning at startup. After upgrading the CRDs,
restart kgateway so that it writes UDPRoute status.

## Listeners and routes

- A UDP listener serves one UDPRoute. UDP has nothing like SNI to tell routes apart, so the oldest
  route by creation timestamp is programmed and the others report `kgateway.dev/Programmed=False`
  with reason `Conflicted`.
- A UDPRoute has one rule. A route with more rules is rejected with `Accepted=False` and reason
  `UnsupportedValue`, because `udp_proxy` cannot express more than one.

## Sessions

`udp_proxy` load balances once per session, not once per datagram.

- A session is the downstream 4-tuple: the client's source IP and port, and the local IP and port
  the datagram arrives on.
- The first datagram of a session picks the host. Every later datagram of the session goes to the
  same host.
- A session ends after 60 seconds without datagrams in either direction.
- When its host is removed from the cluster, for example because a pod goes away, the session ends.
  The next datagram from that client starts a new session and picks a new host.
- When its host is unhealthy, for example because of an active health check or outlier detection,
  the next datagram moves the session to a healthy host if one is available. Without a healthy host
  the session keeps the unhealthy one.

## A single backendRef

The route targets the backend's own cluster, so the backend's policies apply as they do for a
TCPRoute, with these exceptions:

- Ring Hash and Maglev hash on the `udp_proxy` hash policy, not on the cluster's hash policies. A
  BackendConfigPolicy `sourceIP` hash policy is carried over to `udp_proxy`, which pins each client
  IP to a host. Without it, Ring Hash and Maglev pick a host at random.
- Settings that only act on HTTP or TCP have no effect: header and cookie hash policies,
  `sessionPersistence`, `commonHttpProtocolOptions`, `http1ProtocolOptions`, `http2ProtocolOptions`,
  `tls`, `tcpKeepalive`, `upstreamProxyProtocol` and `tunnel`.

## Several backendRefs

`udp_proxy` has no weighted clusters, so a route with several backendRefs targets one synthetic EDS
cluster per route, `udpagg_<namespace>_<name>`. Its endpoints are the weighted union of the
backends' endpoints.

- kgateway builds the union for each Envoy client from that client's endpoints for each backend, as
  the regular endpoint pipeline produces them. Endpoint plugins and per-client endpoint selection
  therefore apply as they do for a single backend, and each endpoint keeps its health status.
- The backendRef weights take precedence over each backend's priorities. All endpoints sit at one
  priority, so locality failover and the preference of a Service `trafficDistribution` do not apply.
  Each backend's share is split over its endpoints in proportion to their own weights.
- A backend with no endpoints, an invalid backendRef and a backend that keeps its endpoints in its own
  cluster (a Static Backend, for example) do not receive traffic. Their share is dropped, not given to
  the other backends, as the UDPRoute spec requires.
- The synthetic cluster carries none of the backends' cluster settings: load balancing policy, health
  checks, outlier detection, circuit breakers, timeouts and the like. It also uses no hash policy.

## Status

When a backend policy setting is not applied, the route stays programmed and reports
`kgateway.dev/Programmed=True` with reason `BackendPolicyNotApplied`. The message names the policy,
the backend and the settings. With several backendRefs, it names every policy attached to a backend,
since none of their cluster settings apply. A policy attached to a backend that one route uses alone
and another route uses with other backends applies to the first route and is reported on the second.
