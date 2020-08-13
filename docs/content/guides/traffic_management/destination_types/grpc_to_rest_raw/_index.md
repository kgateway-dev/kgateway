---
title: gRPC to REST Raw
weight: 135
description: Routing gRPC services to a REST API using raw Envoy config
---

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  proxyNames:
  - gateway-proxy
  ssl: false
  useProxyProto: false
```

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      grpc_json_transcoder:
        proto_descriptor_bin: CqwFChVnb29nbGUvYXBpL2h0dHAucHJvdG8SCmdvb2dsZS5hcGkieQoESHR0cBIqCgVydWxlcxgBIAMoCzIULmdvb2dsZS5hcGkuSHR0cFJ1bGVSBXJ1bGVzEkUKH2Z1bGx5X2RlY29kZV9yZXNlcnZlZF9leHBhbnNpb24YAiABKAhSHGZ1bGx5RGVjb2RlUmVzZXJ2ZWRFeHBhbnNpb24i2gIKCEh0dHBSdWxlEhoKCHNlbGVjdG9yGAEgASgJUghzZWxlY3RvchISCgNnZXQYAiABKAlIAFIDZ2V0EhIKA3B1dBgDIAEoCUgAUgNwdXQSFAoEcG9zdBgEIAEoCUgAUgRwb3N0EhgKBmRlbGV0ZRgFIAEoCUgAUgZkZWxldGUSFgoFcGF0Y2gYBiABKAlIAFIFcGF0Y2gSNwoGY3VzdG9tGAggASgLMh0uZ29vZ2xlLmFwaS5DdXN0b21IdHRwUGF0dGVybkgAUgZjdXN0b20SEgoEYm9keRgHIAEoCVIEYm9keRIjCg1yZXNwb25zZV9ib2R5GAwgASgJUgxyZXNwb25zZUJvZHkSRQoTYWRkaXRpb25hbF9iaW5kaW5ncxgLIAMoCzIULmdvb2dsZS5hcGkuSHR0cFJ1bGVSEmFkZGl0aW9uYWxCaW5kaW5nc0IJCgdwYXR0ZXJuIjsKEUN1c3RvbUh0dHBQYXR0ZXJuEhIKBGtpbmQYASABKAlSBGtpbmQSEgoEcGF0aBgCIAEoCVIEcGF0aEJqCg5jb20uZ29vZ2xlLmFwaUIJSHR0cFByb3RvUAFaQWdvb2dsZS5nb2xhbmcub3JnL2dlbnByb3RvL2dvb2dsZWFwaXMvYXBpL2Fubm90YXRpb25zO2Fubm90YXRpb25z+AEBogIER0FQSWIGcHJvdG8zCps7CiBnb29nbGUvcHJvdG9idWYvZGVzY3JpcHRvci5wcm90bxIPZ29vZ2xlLnByb3RvYnVmIk0KEUZpbGVEZXNjcmlwdG9yU2V0EjgKBGZpbGUYASADKAsyJC5nb29nbGUucHJvdG9idWYuRmlsZURlc2NyaXB0b3JQcm90b1IEZmlsZSLkBAoTRmlsZURlc2NyaXB0b3JQcm90bxISCgRuYW1lGAEgASgJUgRuYW1lEhgKB3BhY2thZ2UYAiABKAlSB3BhY2thZ2USHgoKZGVwZW5kZW5jeRgDIAMoCVIKZGVwZW5kZW5jeRIrChFwdWJsaWNfZGVwZW5kZW5jeRgKIAMoBVIQcHVibGljRGVwZW5kZW5jeRInCg93ZWFrX2RlcGVuZGVuY3kYCyADKAVSDndlYWtEZXBlbmRlbmN5EkMKDG1lc3NhZ2VfdHlwZRgEIAMoCzIgLmdvb2dsZS5wcm90b2J1Zi5EZXNjcmlwdG9yUHJvdG9SC21lc3NhZ2VUeXBlEkEKCWVudW1fdHlwZRgFIAMoCzIkLmdvb2dsZS5wcm90b2J1Zi5FbnVtRGVzY3JpcHRvclByb3RvUghlbnVtVHlwZRJBCgdzZXJ2aWNlGAYgAygLMicuZ29vZ2xlLnByb3RvYnVmLlNlcnZpY2VEZXNjcmlwdG9yUHJvdG9SB3NlcnZpY2USQwoJZXh0ZW5zaW9uGAcgAygLMiUuZ29vZ2xlLnByb3RvYnVmLkZpZWxkRGVzY3JpcHRvclByb3RvUglleHRlbnNpb24SNgoHb3B0aW9ucxgIIAEoCzIcLmdvb2dsZS5wcm90b2J1Zi5GaWxlT3B0aW9uc1IHb3B0aW9ucxJJChBzb3VyY2VfY29kZV9pbmZvGAkgASgLMh8uZ29vZ2xlLnByb3RvYnVmLlNvdXJjZUNvZGVJbmZvUg5zb3VyY2VDb2RlSW5mbxIWCgZzeW50YXgYDCABKAlSBnN5bnRheCK5BgoPRGVzY3JpcHRvclByb3RvEhIKBG5hbWUYASABKAlSBG5hbWUSOwoFZmllbGQYAiADKAsyJS5nb29nbGUucHJvdG9idWYuRmllbGREZXNjcmlwdG9yUHJvdG9SBWZpZWxkEkMKCWV4dGVuc2lvbhgGIAMoCzIlLmdvb2dsZS5wcm90b2J1Zi5GaWVsZERlc2NyaXB0b3JQcm90b1IJZXh0ZW5zaW9uEkEKC25lc3RlZF90eXBlGAMgAygLMiAuZ29vZ2xlLnByb3RvYnVmLkRlc2NyaXB0b3JQcm90b1IKbmVzdGVkVHlwZRJBCgllbnVtX3R5cGUYBCADKAsyJC5nb29nbGUucHJvdG9idWYuRW51bURlc2NyaXB0b3JQcm90b1IIZW51bVR5cGUSWAoPZXh0ZW5zaW9uX3JhbmdlGAUgAygLMi8uZ29vZ2xlLnByb3RvYnVmLkRlc2NyaXB0b3JQcm90by5FeHRlbnNpb25SYW5nZVIOZXh0ZW5zaW9uUmFuZ2USRAoKb25lb2ZfZGVjbBgIIAMoCzIlLmdvb2dsZS5wcm90b2J1Zi5PbmVvZkRlc2NyaXB0b3JQcm90b1IJb25lb2ZEZWNsEjkKB29wdGlvbnMYByABKAsyHy5nb29nbGUucHJvdG9idWYuTWVzc2FnZU9wdGlvbnNSB29wdGlvbnMSVQoOcmVzZXJ2ZWRfcmFuZ2UYCSADKAsyLi5nb29nbGUucHJvdG9idWYuRGVzY3JpcHRvclByb3RvLlJlc2VydmVkUmFuZ2VSDXJlc2VydmVkUmFuZ2USIwoNcmVzZXJ2ZWRfbmFtZRgKIAMoCVIMcmVzZXJ2ZWROYW1lGnoKDkV4dGVuc2lvblJhbmdlEhQKBXN0YXJ0GAEgASgFUgVzdGFydBIQCgNlbmQYAiABKAVSA2VuZBJACgdvcHRpb25zGAMgASgLMiYuZ29vZ2xlLnByb3RvYnVmLkV4dGVuc2lvblJhbmdlT3B0aW9uc1IHb3B0aW9ucxo3Cg1SZXNlcnZlZFJhbmdlEhQKBXN0YXJ0GAEgASgFUgVzdGFydBIQCgNlbmQYAiABKAVSA2VuZCJ8ChVFeHRlbnNpb25SYW5nZU9wdGlvbnMSWAoUdW5pbnRlcnByZXRlZF9vcHRpb24Y5wcgAygLMiQuZ29vZ2xlLnByb3RvYnVmLlVuaW50ZXJwcmV0ZWRPcHRpb25SE3VuaW50ZXJwcmV0ZWRPcHRpb24qCQjoBxCAgICAAiKYBgoURmllbGREZXNjcmlwdG9yUHJvdG8SEgoEbmFtZRgBIAEoCVIEbmFtZRIWCgZudW1iZXIYAyABKAVSBm51bWJlchJBCgVsYWJlbBgEIAEoDjIrLmdvb2dsZS5wcm90b2J1Zi5GaWVsZERlc2NyaXB0b3JQcm90by5MYWJlbFIFbGFiZWwSPgoEdHlwZRgFIAEoDjIqLmdvb2dsZS5wcm90b2J1Zi5GaWVsZERlc2NyaXB0b3JQcm90by5UeXBlUgR0eXBlEhsKCXR5cGVfbmFtZRgGIAEoCVIIdHlwZU5hbWUSGgoIZXh0ZW5kZWUYAiABKAlSCGV4dGVuZGVlEiMKDWRlZmF1bHRfdmFsdWUYByABKAlSDGRlZmF1bHRWYWx1ZRIfCgtvbmVvZl9pbmRleBgJIAEoBVIKb25lb2ZJbmRleBIbCglqc29uX25hbWUYCiABKAlSCGpzb25OYW1lEjcKB29wdGlvbnMYCCABKAsyHS5nb29nbGUucHJvdG9idWYuRmllbGRPcHRpb25zUgdvcHRpb25zIrYCCgRUeXBlEg8KC1RZUEVfRE9VQkxFEAESDgoKVFlQRV9GTE9BVBACEg4KClRZUEVfSU5UNjQQAxIPCgtUWVBFX1VJTlQ2NBAEEg4KClRZUEVfSU5UMzIQBRIQCgxUWVBFX0ZJWEVENjQQBhIQCgxUWVBFX0ZJWEVEMzIQBxINCglUWVBFX0JPT0wQCBIPCgtUWVBFX1NUUklORxAJEg4KClRZUEVfR1JPVVAQChIQCgxUWVBFX01FU1NBR0UQCxIOCgpUWVBFX0JZVEVTEAwSDwoLVFlQRV9VSU5UMzIQDRINCglUWVBFX0VOVU0QDhIRCg1UWVBFX1NGSVhFRDMyEA8SEQoNVFlQRV9TRklYRUQ2NBAQEg8KC1RZUEVfU0lOVDMyEBESDwoLVFlQRV9TSU5UNjQQEiJDCgVMYWJlbBISCg5MQUJFTF9PUFRJT05BTBABEhIKDkxBQkVMX1JFUVVJUkVEEAISEgoOTEFCRUxfUkVQRUFURUQQAyJjChRPbmVvZkRlc2NyaXB0b3JQcm90bxISCgRuYW1lGAEgASgJUgRuYW1lEjcKB29wdGlvbnMYAiABKAsyHS5nb29nbGUucHJvdG9idWYuT25lb2ZPcHRpb25zUgdvcHRpb25zIuMCChNFbnVtRGVzY3JpcHRvclByb3RvEhIKBG5hbWUYASABKAlSBG5hbWUSPwoFdmFsdWUYAiADKAsyKS5nb29nbGUucHJvdG9idWYuRW51bVZhbHVlRGVzY3JpcHRvclByb3RvUgV2YWx1ZRI2CgdvcHRpb25zGAMgASgLMhwuZ29vZ2xlLnByb3RvYnVmLkVudW1PcHRpb25zUgdvcHRpb25zEl0KDnJlc2VydmVkX3JhbmdlGAQgAygLMjYuZ29vZ2xlLnByb3RvYnVmLkVudW1EZXNjcmlwdG9yUHJvdG8uRW51bVJlc2VydmVkUmFuZ2VSDXJlc2VydmVkUmFuZ2USIwoNcmVzZXJ2ZWRfbmFtZRgFIAMoCVIMcmVzZXJ2ZWROYW1lGjsKEUVudW1SZXNlcnZlZFJhbmdlEhQKBXN0YXJ0GAEgASgFUgVzdGFydBIQCgNlbmQYAiABKAVSA2VuZCKDAQoYRW51bVZhbHVlRGVzY3JpcHRvclByb3RvEhIKBG5hbWUYASABKAlSBG5hbWUSFgoGbnVtYmVyGAIgASgFUgZudW1iZXISOwoHb3B0aW9ucxgDIAEoCzIhLmdvb2dsZS5wcm90b2J1Zi5FbnVtVmFsdWVPcHRpb25zUgdvcHRpb25zIqcBChZTZXJ2aWNlRGVzY3JpcHRvclByb3RvEhIKBG5hbWUYASABKAlSBG5hbWUSPgoGbWV0aG9kGAIgAygLMiYuZ29vZ2xlLnByb3RvYnVmLk1ldGhvZERlc2NyaXB0b3JQcm90b1IGbWV0aG9kEjkKB29wdGlvbnMYAyABKAsyHy5nb29nbGUucHJvdG9idWYuU2VydmljZU9wdGlvbnNSB29wdGlvbnMiiQIKFU1ldGhvZERlc2NyaXB0b3JQcm90bxISCgRuYW1lGAEgASgJUgRuYW1lEh0KCmlucHV0X3R5cGUYAiABKAlSCWlucHV0VHlwZRIfCgtvdXRwdXRfdHlwZRgDIAEoCVIKb3V0cHV0VHlwZRI4CgdvcHRpb25zGAQgASgLMh4uZ29vZ2xlLnByb3RvYnVmLk1ldGhvZE9wdGlvbnNSB29wdGlvbnMSMAoQY2xpZW50X3N0cmVhbWluZxgFIAEoCDoFZmFsc2VSD2NsaWVudFN0cmVhbWluZxIwChBzZXJ2ZXJfc3RyZWFtaW5nGAYgASgIOgVmYWxzZVIPc2VydmVyU3RyZWFtaW5nIpIJCgtGaWxlT3B0aW9ucxIhCgxqYXZhX3BhY2thZ2UYASABKAlSC2phdmFQYWNrYWdlEjAKFGphdmFfb3V0ZXJfY2xhc3NuYW1lGAggASgJUhJqYXZhT3V0ZXJDbGFzc25hbWUSNQoTamF2YV9tdWx0aXBsZV9maWxlcxgKIAEoCDoFZmFsc2VSEWphdmFNdWx0aXBsZUZpbGVzEkQKHWphdmFfZ2VuZXJhdGVfZXF1YWxzX2FuZF9oYXNoGBQgASgIQgIYAVIZamF2YUdlbmVyYXRlRXF1YWxzQW5kSGFzaBI6ChZqYXZhX3N0cmluZ19jaGVja191dGY4GBsgASgIOgVmYWxzZVITamF2YVN0cmluZ0NoZWNrVXRmOBJTCgxvcHRpbWl6ZV9mb3IYCSABKA4yKS5nb29nbGUucHJvdG9idWYuRmlsZU9wdGlvbnMuT3B0aW1pemVNb2RlOgVTUEVFRFILb3B0aW1pemVGb3ISHQoKZ29fcGFja2FnZRgLIAEoCVIJZ29QYWNrYWdlEjUKE2NjX2dlbmVyaWNfc2VydmljZXMYECABKAg6BWZhbHNlUhFjY0dlbmVyaWNTZXJ2aWNlcxI5ChVqYXZhX2dlbmVyaWNfc2VydmljZXMYESABKAg6BWZhbHNlUhNqYXZhR2VuZXJpY1NlcnZpY2VzEjUKE3B5X2dlbmVyaWNfc2VydmljZXMYEiABKAg6BWZhbHNlUhFweUdlbmVyaWNTZXJ2aWNlcxI3ChRwaHBfZ2VuZXJpY19zZXJ2aWNlcxgqIAEoCDoFZmFsc2VSEnBocEdlbmVyaWNTZXJ2aWNlcxIlCgpkZXByZWNhdGVkGBcgASgIOgVmYWxzZVIKZGVwcmVjYXRlZBIvChBjY19lbmFibGVfYXJlbmFzGB8gASgIOgVmYWxzZVIOY2NFbmFibGVBcmVuYXMSKgoRb2JqY19jbGFzc19wcmVmaXgYJCABKAlSD29iamNDbGFzc1ByZWZpeBIpChBjc2hhcnBfbmFtZXNwYWNlGCUgASgJUg9jc2hhcnBOYW1lc3BhY2USIQoMc3dpZnRfcHJlZml4GCcgASgJUgtzd2lmdFByZWZpeBIoChBwaHBfY2xhc3NfcHJlZml4GCggASgJUg5waHBDbGFzc1ByZWZpeBIjCg1waHBfbmFtZXNwYWNlGCkgASgJUgxwaHBOYW1lc3BhY2USNAoWcGhwX21ldGFkYXRhX25hbWVzcGFjZRgsIAEoCVIUcGhwTWV0YWRhdGFOYW1lc3BhY2USIQoMcnVieV9wYWNrYWdlGC0gASgJUgtydWJ5UGFja2FnZRJYChR1bmludGVycHJldGVkX29wdGlvbhjnByADKAsyJC5nb29nbGUucHJvdG9idWYuVW5pbnRlcnByZXRlZE9wdGlvblITdW5pbnRlcnByZXRlZE9wdGlvbiI6CgxPcHRpbWl6ZU1vZGUSCQoFU1BFRUQQARINCglDT0RFX1NJWkUQAhIQCgxMSVRFX1JVTlRJTUUQAyoJCOgHEICAgIACSgQIJhAnItECCg5NZXNzYWdlT3B0aW9ucxI8ChdtZXNzYWdlX3NldF93aXJlX2Zvcm1hdBgBIAEoCDoFZmFsc2VSFG1lc3NhZ2VTZXRXaXJlRm9ybWF0EkwKH25vX3N0YW5kYXJkX2Rlc2NyaXB0b3JfYWNjZXNzb3IYAiABKAg6BWZhbHNlUhxub1N0YW5kYXJkRGVzY3JpcHRvckFjY2Vzc29yEiUKCmRlcHJlY2F0ZWQYAyABKAg6BWZhbHNlUgpkZXByZWNhdGVkEhsKCW1hcF9lbnRyeRgHIAEoCFIIbWFwRW50cnkSWAoUdW5pbnRlcnByZXRlZF9vcHRpb24Y5wcgAygLMiQuZ29vZ2xlLnByb3RvYnVmLlVuaW50ZXJwcmV0ZWRPcHRpb25SE3VuaW50ZXJwcmV0ZWRPcHRpb24qCQjoBxCAgICAAkoECAgQCUoECAkQCiLiAwoMRmllbGRPcHRpb25zEkEKBWN0eXBlGAEgASgOMiMuZ29vZ2xlLnByb3RvYnVmLkZpZWxkT3B0aW9ucy5DVHlwZToGU1RSSU5HUgVjdHlwZRIWCgZwYWNrZWQYAiABKAhSBnBhY2tlZBJHCgZqc3R5cGUYBiABKA4yJC5nb29nbGUucHJvdG9idWYuRmllbGRPcHRpb25zLkpTVHlwZToJSlNfTk9STUFMUgZqc3R5cGUSGQoEbGF6eRgFIAEoCDoFZmFsc2VSBGxhenkSJQoKZGVwcmVjYXRlZBgDIAEoCDoFZmFsc2VSCmRlcHJlY2F0ZWQSGQoEd2VhaxgKIAEoCDoFZmFsc2VSBHdlYWsSWAoUdW5pbnRlcnByZXRlZF9vcHRpb24Y5wcgAygLMiQuZ29vZ2xlLnByb3RvYnVmLlVuaW50ZXJwcmV0ZWRPcHRpb25SE3VuaW50ZXJwcmV0ZWRPcHRpb24iLwoFQ1R5cGUSCgoGU1RSSU5HEAASCAoEQ09SRBABEhAKDFNUUklOR19QSUVDRRACIjUKBkpTVHlwZRINCglKU19OT1JNQUwQABINCglKU19TVFJJTkcQARINCglKU19OVU1CRVIQAioJCOgHEICAgIACSgQIBBAFInMKDE9uZW9mT3B0aW9ucxJYChR1bmludGVycHJldGVkX29wdGlvbhjnByADKAsyJC5nb29nbGUucHJvdG9idWYuVW5pbnRlcnByZXRlZE9wdGlvblITdW5pbnRlcnByZXRlZE9wdGlvbioJCOgHEICAgIACIsABCgtFbnVtT3B0aW9ucxIfCgthbGxvd19hbGlhcxgCIAEoCFIKYWxsb3dBbGlhcxIlCgpkZXByZWNhdGVkGAMgASgIOgVmYWxzZVIKZGVwcmVjYXRlZBJYChR1bmludGVycHJldGVkX29wdGlvbhjnByADKAsyJC5nb29nbGUucHJvdG9idWYuVW5pbnRlcnByZXRlZE9wdGlvblITdW5pbnRlcnByZXRlZE9wdGlvbioJCOgHEICAgIACSgQIBRAGIp4BChBFbnVtVmFsdWVPcHRpb25zEiUKCmRlcHJlY2F0ZWQYASABKAg6BWZhbHNlUgpkZXByZWNhdGVkElgKFHVuaW50ZXJwcmV0ZWRfb3B0aW9uGOcHIAMoCzIkLmdvb2dsZS5wcm90b2J1Zi5VbmludGVycHJldGVkT3B0aW9uUhN1bmludGVycHJldGVkT3B0aW9uKgkI6AcQgICAgAIinAEKDlNlcnZpY2VPcHRpb25zEiUKCmRlcHJlY2F0ZWQYISABKAg6BWZhbHNlUgpkZXByZWNhdGVkElgKFHVuaW50ZXJwcmV0ZWRfb3B0aW9uGOcHIAMoCzIkLmdvb2dsZS5wcm90b2J1Zi5VbmludGVycHJldGVkT3B0aW9uUhN1bmludGVycHJldGVkT3B0aW9uKgkI6AcQgICAgAIi4AIKDU1ldGhvZE9wdGlvbnMSJQoKZGVwcmVjYXRlZBghIAEoCDoFZmFsc2VSCmRlcHJlY2F0ZWQScQoRaWRlbXBvdGVuY3lfbGV2ZWwYIiABKA4yLy5nb29nbGUucHJvdG9idWYuTWV0aG9kT3B0aW9ucy5JZGVtcG90ZW5jeUxldmVsOhNJREVNUE9URU5DWV9VTktOT1dOUhBpZGVtcG90ZW5jeUxldmVsElgKFHVuaW50ZXJwcmV0ZWRfb3B0aW9uGOcHIAMoCzIkLmdvb2dsZS5wcm90b2J1Zi5VbmludGVycHJldGVkT3B0aW9uUhN1bmludGVycHJldGVkT3B0aW9uIlAKEElkZW1wb3RlbmN5TGV2ZWwSFwoTSURFTVBPVEVOQ1lfVU5LTk9XThAAEhMKD05PX1NJREVfRUZGRUNUUxABEg4KCklERU1QT1RFTlQQAioJCOgHEICAgIACIpoDChNVbmludGVycHJldGVkT3B0aW9uEkEKBG5hbWUYAiADKAsyLS5nb29nbGUucHJvdG9idWYuVW5pbnRlcnByZXRlZE9wdGlvbi5OYW1lUGFydFIEbmFtZRIpChBpZGVudGlmaWVyX3ZhbHVlGAMgASgJUg9pZGVudGlmaWVyVmFsdWUSLAoScG9zaXRpdmVfaW50X3ZhbHVlGAQgASgEUhBwb3NpdGl2ZUludFZhbHVlEiwKEm5lZ2F0aXZlX2ludF92YWx1ZRgFIAEoA1IQbmVnYXRpdmVJbnRWYWx1ZRIhCgxkb3VibGVfdmFsdWUYBiABKAFSC2RvdWJsZVZhbHVlEiEKDHN0cmluZ192YWx1ZRgHIAEoDFILc3RyaW5nVmFsdWUSJwoPYWdncmVnYXRlX3ZhbHVlGAggASgJUg5hZ2dyZWdhdGVWYWx1ZRpKCghOYW1lUGFydBIbCgluYW1lX3BhcnQYASACKAlSCG5hbWVQYXJ0EiEKDGlzX2V4dGVuc2lvbhgCIAIoCFILaXNFeHRlbnNpb24ipwIKDlNvdXJjZUNvZGVJbmZvEkQKCGxvY2F0aW9uGAEgAygLMiguZ29vZ2xlLnByb3RvYnVmLlNvdXJjZUNvZGVJbmZvLkxvY2F0aW9uUghsb2NhdGlvbhrOAQoITG9jYXRpb24SFgoEcGF0aBgBIAMoBUICEAFSBHBhdGgSFgoEc3BhbhgCIAMoBUICEAFSBHNwYW4SKQoQbGVhZGluZ19jb21tZW50cxgDIAEoCVIPbGVhZGluZ0NvbW1lbnRzEisKEXRyYWlsaW5nX2NvbW1lbnRzGAQgASgJUhB0cmFpbGluZ0NvbW1lbnRzEjoKGWxlYWRpbmdfZGV0YWNoZWRfY29tbWVudHMYBiADKAlSF2xlYWRpbmdEZXRhY2hlZENvbW1lbnRzItEBChFHZW5lcmF0ZWRDb2RlSW5mbxJNCgphbm5vdGF0aW9uGAEgAygLMi0uZ29vZ2xlLnByb3RvYnVmLkdlbmVyYXRlZENvZGVJbmZvLkFubm90YXRpb25SCmFubm90YXRpb24abQoKQW5ub3RhdGlvbhIWCgRwYXRoGAEgAygFQgIQAVIEcGF0aBIfCgtzb3VyY2VfZmlsZRgCIAEoCVIKc291cmNlRmlsZRIUCgViZWdpbhgDIAEoBVIFYmVnaW4SEAoDZW5kGAQgASgFUgNlbmRCjwEKE2NvbS5nb29nbGUucHJvdG9idWZCEERlc2NyaXB0b3JQcm90b3NIAVo+Z2l0aHViLmNvbS9nb2xhbmcvcHJvdG9idWYvcHJvdG9jLWdlbi1nby9kZXNjcmlwdG9yO2Rlc2NyaXB0b3L4AQGiAgNHUEKqAhpHb29nbGUuUHJvdG9idWYuUmVmbGVjdGlvbgqoAgocZ29vZ2xlL2FwaS9hbm5vdGF0aW9ucy5wcm90bxIKZ29vZ2xlLmFwaRoVZ29vZ2xlL2FwaS9odHRwLnByb3RvGiBnb29nbGUvcHJvdG9idWYvZGVzY3JpcHRvci5wcm90bzpLCgRodHRwEh4uZ29vZ2xlLnByb3RvYnVmLk1ldGhvZE9wdGlvbnMYsMq8IiABKAsyFC5nb29nbGUuYXBpLkh0dHBSdWxlUgRodHRwQm4KDmNvbS5nb29nbGUuYXBpQhBBbm5vdGF0aW9uc1Byb3RvUAFaQWdvb2dsZS5nb2xhbmcub3JnL2dlbnByb3RvL2dvb2dsZWFwaXMvYXBpL2Fubm90YXRpb25zO2Fubm90YXRpb25zogIER0FQSWIGcHJvdG8zCp4KChBwcm90by9maWxlLnByb3RvEhBzb2xvLmV4YW1wbGVzLnYxGhxnb29nbGUvYXBpL2Fubm90YXRpb25zLnByb3RvIlIKBEl0ZW0SEgoEbmFtZRgBIAEoCVIEbmFtZRIgCgtkZXNjcmlwdGlvbhgCIAEoCVILZGVzY3JpcHRpb24SFAoFcHJpY2UYAyABKAFSBXByaWNlIj0KD0dldEl0ZW1SZXNwb25zZRIqCgRpdGVtGAEgASgLMhYuc29sby5leGFtcGxlcy52MS5JdGVtUgRpdGVtIiQKDkdldEl0ZW1SZXF1ZXN0EhIKBG5hbWUYASABKAlSBG5hbWUiQQoRTGlzdEl0ZW1zUmVzcG9uc2USLAoFaXRlbXMYASADKAsyFi5zb2xvLmV4YW1wbGVzLnYxLkl0ZW1SBWl0ZW1zIhIKEExpc3RJdGVtc1JlcXVlc3QiQAoSQ3JlYXRlSXRlbVJlc3BvbnNlEioKBGl0ZW0YASABKAsyFi5zb2xvLmV4YW1wbGVzLnYxLkl0ZW1SBGl0ZW0iPwoRQ3JlYXRlSXRlbVJlcXVlc3QSKgoEaXRlbRgBIAEoCzIWLnNvbG8uZXhhbXBsZXMudjEuSXRlbVIEaXRlbSJAChJEZWxldGVJdGVtUmVzcG9uc2USKgoEaXRlbRgBIAEoCzIWLnNvbG8uZXhhbXBsZXMudjEuSXRlbVIEaXRlbSInChFEZWxldGVJdGVtUmVxdWVzdBISCgRuYW1lGAEgASgJUgRuYW1lMsoFCgxTdG9yZVNlcnZpY2USsAEKCkNyZWF0ZUl0ZW0SIy5zb2xvLmV4YW1wbGVzLnYxLkNyZWF0ZUl0ZW1SZXF1ZXN0GiQuc29sby5leGFtcGxlcy52MS5DcmVhdGVJdGVtUmVzcG9uc2UiV4LT5JMCUToBKiJMLzhjYTEzNmQ5L2RlZmF1bHQtZ3JwY3N0b3JlLWRlbW8tODAvc29sby5leGFtcGxlcy52MS5TdG9yZVNlcnZpY2UvQ3JlYXRlSXRlbRKsAQoJTGlzdEl0ZW1zEiIuc29sby5leGFtcGxlcy52MS5MaXN0SXRlbXNSZXF1ZXN0GiMuc29sby5leGFtcGxlcy52MS5MaXN0SXRlbXNSZXNwb25zZSJWgtPkkwJQOgEqIksvOGNhMTM2ZDkvZGVmYXVsdC1ncnBjc3RvcmUtZGVtby04MC9zb2xvLmV4YW1wbGVzLnYxLlN0b3JlU2VydmljZS9MaXN0SXRlbXMSsAEKCkRlbGV0ZUl0ZW0SIy5zb2xvLmV4YW1wbGVzLnYxLkRlbGV0ZUl0ZW1SZXF1ZXN0GiQuc29sby5leGFtcGxlcy52MS5EZWxldGVJdGVtUmVzcG9uc2UiV4LT5JMCUToBKiJMLzhjYTEzNmQ5L2RlZmF1bHQtZ3JwY3N0b3JlLWRlbW8tODAvc29sby5leGFtcGxlcy52MS5TdG9yZVNlcnZpY2UvRGVsZXRlSXRlbRKkAQoHR2V0SXRlbRIgLnNvbG8uZXhhbXBsZXMudjEuR2V0SXRlbVJlcXVlc3QaIS5zb2xvLmV4YW1wbGVzLnYxLkdldEl0ZW1SZXNwb25zZSJUgtPkkwJOOgEqIkkvOGNhMTM2ZDkvZGVmYXVsdC1ncnBjc3RvcmUtZGVtby04MC9zb2xvLmV4YW1wbGVzLnYxLlN0b3JlU2VydmljZS9HZXRJdGVtQgdaBXByb3RvYgZwcm90bzM=
        services:
          - solo.examples.v1.StoreService
        match_incoming_request_route: false
  proxyNames:
  - gateway-proxy
  ssl: false
  useProxyProto: false
```

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default2
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - foo.example.com
    routes:
    - matchers:
       - methods:
         - GET
         - POST
         prefix: /
      routeAction:
       single:
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
```

confirmed that they can coexist (both on the filter chain at same time):
```shell script
curl -v -XPOST -H "Host: foo.example.com" $(glooctl proxy url)/8ca136d9/default-grpcstore-demo-80/solo.examples.v1.StoreService/ListItems
```

A growing trend is to use gRPC internally as the communication protocol between micro-services. This has quite a few advantages. Some of those are:

1. Client and server stubs are auto generated
1. Efficient binary protocol (Google's protobufs)
1. Cross-language support as client and server libraries are available in many languages
1. HTTP based which plays well with existing firewalls and load balancers
1. Well supported with tooling around observability

While gRPC works great for internal micro-services, it may be desirable to have the internet facing API be a JSON\REST style API. This can happen for many reasons. among which are:

1. Keeping the API backwards compatible
1. Making the API more Web friendly
1. Supporting low-end devices such as IoT where gRPC is not supported.

Gloo allows you to define JSON/REST to your gRPC API so you can have the best of both worlds - outwards facing REST API and an internal gRPC API with no extra code.

With Gloo, there is no need to annotate your proto definitions with the `google.api.http` options. A simple gRPC proto will work.

---

## Overview

In this guide we will deploy a gRPC micro-service and transform its gRPC API to a REST API via Gloo.

Usually, to understand the details of the binary protobuf, a protobuf descriptor is needed. As this micro-service is built with server reflection enabled; together with Gloo's automatic function discovery functionality the required protobuf descriptor will be automatically discovered.

In this guide we are going to:

1. Deploy a gRPC demo service
1. Verify that the gRPC descriptors were indeed discovered
1. Add a Virtual Service creating a REST API that maps to the gRPC API
1. Verify that everything is working as expected

Let's get started!

### Prereqs

Install Gloo with Function Discovery Service (FDS) [blacklist mode]({{< versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#configuring-the-fdsmode-setting" >}}) enabled

---

## Deploy the demo gRPC store

Create a deployment and a service:

```shell
kubectl create deployment grpcstore-demo --image=docker.io/soloio/grpcstore-demo
kubectl expose deployment grpcstore-demo --port 80 --target-port=8080
```

### Verify that gRPC functions were discovered
After a few seconds Gloo should have discovered the service with it's proto descriptor:

```shell
kubectl get upstream -n gloo-system default-grpcstore-demo-80 -o yaml
```

You should see output similar to this:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: grpcstore-demo
    discovered_by: kubernetesplugin
  name: default-grpcstore-demo-80
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: grpcstore-demo
    serviceName: grpcstore-demo
    serviceNamespace: default
    servicePort: 80
    serviceSpec:
      grpc:
        descriptors: Q3F3RkNoVm5iMjluYkdVdllYQnBMMmgwZE…bTkwYnpNPQ==
        grpcServices:
        - functionNames:
          - CreateItem
          - ListItems
          - DeleteItem
          - GetItem
          packageName: solo.examples.v1
          serviceName: StoreService
status:
  reported_by: gloo
  state: 1

```

{{% notice note %}}
The descriptors field above was truncated for brevity.
{{% /notice %}}

As you can see Gloo's function discovery detected the gRPC functions on that service. 

### Create a REST to gRPC translation

Now we are ready to create the external REST to gRPC API. Please run the following command:

```shell
kubectl create -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    routes:
    - matchers:
       - methods:
         - GET
         prefix: /items/
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: GetItem
             package: solo.examples.v1
             parameters:
               path: /items/{name}
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - DELETE
         prefix: /items/
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: DeleteItem
             package: solo.examples.v1
             parameters:
               path: /items/{name}
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - GET
         exact: /items
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: ListItems
             package: solo.examples.v1
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - POST
         exact: /items
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: CreateItem
             package: solo.examples.v1
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
EOF
```

An explanation for the Virtual Service above:
We have defined four routes. Each route uses a {{< protobuf name="grpc.options.gloo.solo.io.DestinationSpec" display="gRPC destinationSpec" >}} to define REST routes to a gRPC service. When translating a REST API to a gRPC API the JSON body is automatically used to fill in the proto message fields. If you have some parameters in the path or in headers, your can specify them using the {{< protobuf name="transformation.options.gloo.solo.io.Parameters" display="parameters">}}  block in the {{< protobuf name="grpc.options.gloo.solo.io.DestinationSpec" display="gRPC destinationSpec">}} (as done in the route to `GetItem` and `DeleteItem`). We use HTTP method matching to make sure that our API adheres to the REST semantics. Note that the routes for `CreateItem` and `ListItems` are defined for the exact path `/items` (i.e. no trailing slash).

### Test

To test, we can use `curl` to issue queries to our new REST API:

```shell
URL=$(glooctl proxy url)
# Create an item in the store.
curl $URL/items -d '{"item":{"name":"item1"}}'
# List all items in the store. You should see an object with a list containing the item created above. 
curl $URL/items
# Access a specific item. You should see the item as a single object.
curl $URL/items/item1
# Delete the item created.
curl $URL/items/item1 -XDELETE
# No items - this will return an empty object.
curl $URL/items
```

---

## Conclusion

In this guide we have deployed a gRPC micro-service and created an external REST API that translates to the gRPC API via Gloo. This allows you to enjoy the benefits of using gRPC for your microservices while still having a traditional REST API without the need to maintain two sets of code. 

### Next Steps

Learn more about how Gloo handles [gRPC for web clients]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}). Gloo can also use a [REST endpoint]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/rest_endpoint/" %}}) as an Upstream. Our [function discovery guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/" %}}) covers how to set up the Function Discovery Service (FDS) for a Swagger document or gRPC service.
