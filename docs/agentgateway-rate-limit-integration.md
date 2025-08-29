# AgentGateway Rate Limiting Integration

This document outlines the rate limiting implementation using the existing agentgateway API.

## Current Status

✅ **Rate limiting support is already available** in agentgateway v0.7.6+ and we're using it properly!

The implementation now uses the native `PolicySpec_LocalRateLimit_` type that's already available in the agentgateway API.

## Available Rate Limiting Types

### 1. Local Rate Limiting

The agentgateway API provides `PolicySpec_LocalRateLimit_` with the following structure:

```go
type PolicySpec_LocalRateLimit struct {
    MaxTokens     uint64                                    // Maximum tokens in bucket
    TokensPerFill uint64                                    // Tokens added per fill interval
    FillInterval  *duration.Duration                        // Time between token fills
    Type          PolicySpec_LocalRateLimit_Type            // Rate limiting type (REQUEST)
}
```

### 2. Rate Limiting Types

```go
type PolicySpec_LocalRateLimit_Type int32

const (
    PolicySpec_LocalRateLimit_REQUEST PolicySpec_LocalRateLimit_Type = 0
    // Additional types may be available in future versions
)
```

## Implementation

The current implementation in kgateway properly translates the kgateway `LocalRateLimitPolicy` to the agentgateway `PolicySpec_LocalRateLimit_`:

```go
Kind: &api.PolicySpec_LocalRateLimit_{
    LocalRateLimit: &api.PolicySpec_LocalRateLimit{
        MaxTokens:     uint64(tokenBucket.MaxTokens),
        TokensPerFill: uint64(ptr.Deref(tokenBucket.TokensPerFill, 1)),
        FillInterval:  &durationpb.Duration{Seconds: int64(fillIntervalSeconds)},
        Type:          api.PolicySpec_LocalRateLimit_REQUEST,
    },
},
```

## Benefits of Using Native Support

1. **Type Safety**: Proper protobuf types with compile-time validation
2. **Performance**: Native implementation in agentgateway data plane
3. **Maintainability**: No placeholder workarounds or metadata hacks
4. **Consistency**: Follows the same pattern as other policy types
5. **Future-Proof**: Will automatically benefit from agentgateway improvements

## Configuration Mapping

| kgateway Field | agentgateway Field | Type | Notes |
|----------------|-------------------|------|-------|
| `maxTokens` | `MaxTokens` | `uint64` | Direct mapping |
| `tokensPerFill` | `TokensPerFill` | `uint64` | Direct mapping |
| `fillInterval` | `FillInterval` | `duration.Duration` | Converted from metav1.Duration |

## Future Enhancements

When agentgateway adds global rate limiting support, we can easily extend the implementation to include:

1. **Global Rate Limiting**: External rate limit service integration
2. **Additional Rate Limiting Types**: Beyond just REQUEST-based limiting
3. **Advanced Configuration**: More sophisticated rate limiting algorithms

## Conclusion

The rate limiting implementation is now properly integrated using the native agentgateway API. No placeholder code or workarounds are needed - we're using the proper, type-safe implementation that was already available.

This provides a solid foundation for local rate limiting and can be easily extended when global rate limiting support becomes available in future agentgateway versions.
