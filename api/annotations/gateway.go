package annotations

// PerConnectionBufferLimit is the annotation key for the per connection buffer limit.
// It is used to set the per connection buffer limit for the gateway.
// The value is a string representing the limit, e.g "64Ki".
// The limit is applied to all listeners in the gateway.
const PerConnectionBufferLimit = "kgateway.dev/per-connection-buffer-limit"
