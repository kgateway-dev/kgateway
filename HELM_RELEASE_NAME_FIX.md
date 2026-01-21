# Fix for Issue #13347: Gateways with Long Names Don't Get Programmed

## Problem Description

The original issue occurred because kgateway directly used the Gateway name as the Helm release name without considering Helm's 53-character limit. This caused failures when Gateway names exceeded this limit, resulting in:

- Gateway showing "Programmed" status as "True" (misleading)
- Actual proxy deployment failing silently
- Helm release creation failing with validation error

## Root Cause

In `pkg/kgateway/deployer/gateway_parameters.go`, the `GatewayReleaseNameAndNamespace` function simply returned `obj.GetName()` as the release name:

```go
func GatewayReleaseNameAndNamespace(obj client.Object) (string, string) {
    return obj.GetName(), obj.GetNamespace()  // No length validation!
}
```

## Solution Implementation

### 1. Enhanced Name Generation Function

The fix introduces a robust `generateHelmReleaseName` function that:

- **Preserves short names**: Names ≤53 chars and already valid are used as-is
- **Truncates long names**: Intelligently shortens names while maintaining readability
- **Prevents collisions**: Uses SHA256 hash of full name + namespace for uniqueness
- **Ensures validity**: Validates against Helm's regex pattern

### 2. Algorithm Details

```
For names requiring truncation:
1. Calculate available space: 53 - 8 (hash) - 1 (separator) = 44 chars
2. Truncate gateway name to 44 chars
3. Remove trailing hyphens to ensure validity
4. Generate 8-char hash from "gatewayName.namespace"
5. Combine: "truncated-name-12345678"
6. Validate result and use fallback if needed
```

### 3. Key Features

- **Deterministic**: Same input always produces same output
- **Collision-resistant**: Hash includes namespace for uniqueness
- **Helm-compliant**: Matches regex `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
- **Readable**: Preserves meaningful prefix when possible
- **Logged**: Debug/warn logging for troubleshooting

### 4. Examples

| Gateway Name | Namespace | Release Name | Notes |
|--------------|-----------|--------------|-------|
| `my-gateway` | `default` | `my-gateway` | Short name preserved |
| `looooooooooooooooooooooooooooooooooooooooooooooooooooong-name` | `default` | `loooooooooooooooooooooooooooooooooooooooooo-a1b2c3d4` | Truncated + hash |
| `very-very-very-very-very-very-very-very-very-long-gateway-name` | `production` | `very-very-very-very-very-very-very-very-ver-e5f6g7h8` | Different hash for different namespace |

## Testing

### Test Coverage

The implementation includes comprehensive tests covering:

- Short names (preserved as-is)
- Long names (truncated with hash)
- Edge cases (exactly 53 chars, ending with hyphens)
- Uniqueness (different inputs produce different outputs)
- Determinism (same input produces same output)
- Helm validation compliance

### Manual Testing

To test the fix with the original issue case:

```bash
# Apply the problematic gateway
kubectl apply -f reproduce_issue.yaml

# Check the generated helm release name in logs
kubectl logs -n kgateway-system deployment/kgateway-controller | grep "Generated truncated helm release name"
```

## Backward Compatibility

- **Existing short names**: No change in behavior
- **Existing long names**: Will now work instead of failing
- **No breaking changes**: All existing functionality preserved

## Performance Impact

- **Minimal overhead**: Hash calculation only for long names
- **O(1) complexity**: Constant time regardless of name length
- **Memory efficient**: No additional storage required

## Security Considerations

- **Hash collision resistance**: SHA256 provides strong collision resistance
- **Deterministic behavior**: No random elements that could cause inconsistency
- **Input validation**: Proper regex validation prevents injection

## Monitoring and Debugging

The fix includes structured logging:

```go
slog.Debug("Generated truncated helm release name",
    "gateway_name", gatewayName,
    "namespace", namespace,
    "release_name", releaseName,
    "original_length", len(gatewayName),
    "final_length", len(releaseName),
)
```

This helps operators understand when and why names are being truncated.

## Future Considerations

1. **Metrics**: Could add metrics for truncated names
2. **Warnings**: Could emit Kubernetes events when names are truncated
3. **Configuration**: Could make hash length configurable if needed
4. **Validation**: Could add admission webhook to warn about long names

## Files Modified

1. `pkg/kgateway/deployer/gateway_parameters.go`:
   - Added `generateHelmReleaseName` function
   - Added `isValidHelmReleaseName` function
   - Updated `GatewayReleaseNameAndNamespace` to use new logic
   - Added necessary imports (crypto/sha256, encoding/hex, regexp)

2. `pkg/kgateway/deployer/gateway_parameters_test.go`:
   - Comprehensive test suite for the new functionality
   - Edge case testing and validation

## Verification

The fix can be verified by:

1. **Unit tests**: Run `go test ./pkg/kgateway/deployer`
2. **Integration test**: Apply the `reproduce_issue.yaml` gateway
3. **Log inspection**: Check for successful helm release creation
4. **Status verification**: Confirm gateway shows proper "Programmed" status

This fix resolves issue #13347 by ensuring all gateway names, regardless of length, can be successfully converted to valid Helm release names while maintaining uniqueness and readability.