package assertions

// WebhookReject is a well-known string that should be placed in the filename of any manifest which should be rejected
// by the Gloo Gateway validating webhook if it is enabled and configured to reject on errors. This acts as signal
// for any assertion which needs to know if the manifest is expected to not be admitted to the cluster.
const WebhookReject = "webhook-reject"
