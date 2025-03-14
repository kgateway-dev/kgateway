import gzip
from api.kgateway.policy.ai import authtoken
from api.envoy.config.core.v3 import base_pb2 as base_pb2
from api.envoy.service.ext_proc.v3 import external_processor_pb2
from api.envoy.type.v3 import http_status_pb2
from typing import Optional, Mapping, Union
from google.protobuf import struct_pb2


def get_auth_token(
    auth: authtoken.SingleAuthToken,
    headers: base_pb2.HeaderMap,
    default: str,
) -> str:
    value = ""
    if auth.kind == authtoken.SingleAuthTokenKind.INLINE:
        value: str = auth.inline
        pass
    elif auth.kind == authtoken.SingleAuthTokenKind.PASSTHROUGH:
        # Get from headers
        value = get_http_header(headers, "Authorization").removeprefix("Bearer ")
    else:
        raise ValueError(f"Unknown auth token source {auth}")
    if value == "":
        value = default
    return value


def get_http_header(headers: base_pb2.HeaderMap, header_name: str) -> str:
    lowered_header = header_name.lower()
    for i in range(headers.headers.__len__()):
        if headers.headers[i].key.lower() == lowered_header:
            return headers.headers[i].raw_value.decode("utf-8")
    return "unknown"


def extproc_clear_request_body() -> external_processor_pb2.ProcessingResponse:
    """
    construct an ext_proc response to clear the request body
    This can be used as response for both request extproc calls only
    """
    return external_processor_pb2.ProcessingResponse(
        request_body=external_processor_pb2.BodyResponse(
            response=external_processor_pb2.CommonResponse(
                body_mutation=external_processor_pb2.BodyMutation(clear_body=True),
            )
        ),
    )


def extproc_clear_response_body() -> external_processor_pb2.ProcessingResponse:
    """
    construct an ext_proc response to clear the response body
    This can be used as response for both response extproc calls only
    """
    return external_processor_pb2.ProcessingResponse(
        response_body=external_processor_pb2.BodyResponse(
            response=external_processor_pb2.CommonResponse(
                body_mutation=external_processor_pb2.BodyMutation(clear_body=True),
            )
        ),
    )


def extproc_new_response_body(
    content_encoding: str,
    body: bytes,
    dynamic_metadata: Optional[Union[struct_pb2.Struct, Mapping]] = None,
) -> external_processor_pb2.ProcessingResponse:
    """
    construct an extproc response to replace the original response body
    """
    return external_processor_pb2.ProcessingResponse(
        response_body=external_processor_pb2.BodyResponse(
            response=external_processor_pb2.CommonResponse(
                body_mutation=external_processor_pb2.BodyMutation(
                    body=(gzip.compress(body) if content_encoding == "gzip" else body),
                ),
            ),
        ),
        dynamic_metadata=dynamic_metadata,
    )


def extproc_immediate_response(
    status_code: int,
    body: str = "",
    details: str
    | None = None,  # details that goes into %RESPONSE_CODE_DETAILS% which can be used in logs
) -> external_processor_pb2.ProcessingResponse:
    """
    construct an extproc response to instruct envoy to return the specified response immediately
    """
    return external_processor_pb2.ProcessingResponse(
        immediate_response=external_processor_pb2.ImmediateResponse(
            status=dict(code=map_int_to_grpc_status_code(status_code)),
            body=body.encode("utf-8"),
            details=details,
        ),
    )


status_code_map: dict[int, http_status_pb2.StatusCode] = {
    100: http_status_pb2.Continue,
    200: http_status_pb2.OK,
    201: http_status_pb2.Created,
    202: http_status_pb2.Accepted,
    203: http_status_pb2.NonAuthoritativeInformation,
    204: http_status_pb2.NoContent,
    205: http_status_pb2.ResetContent,
    206: http_status_pb2.PartialContent,
    207: http_status_pb2.MultiStatus,
    208: http_status_pb2.AlreadyReported,
    226: http_status_pb2.IMUsed,
    300: http_status_pb2.MultipleChoices,
    301: http_status_pb2.MovedPermanently,
    302: http_status_pb2.Found,
    303: http_status_pb2.SeeOther,
    304: http_status_pb2.NotModified,
    305: http_status_pb2.UseProxy,
    307: http_status_pb2.TemporaryRedirect,
    308: http_status_pb2.PermanentRedirect,
    400: http_status_pb2.BadRequest,
    401: http_status_pb2.Unauthorized,
    402: http_status_pb2.PaymentRequired,
    403: http_status_pb2.Forbidden,
    404: http_status_pb2.NotFound,
    405: http_status_pb2.MethodNotAllowed,
    406: http_status_pb2.NotAcceptable,
    407: http_status_pb2.ProxyAuthenticationRequired,
    408: http_status_pb2.RequestTimeout,
    409: http_status_pb2.Conflict,
    410: http_status_pb2.Gone,
    411: http_status_pb2.LengthRequired,
    412: http_status_pb2.PreconditionFailed,
    413: http_status_pb2.PayloadTooLarge,
    414: http_status_pb2.URITooLong,
    415: http_status_pb2.UnsupportedMediaType,
    416: http_status_pb2.RangeNotSatisfiable,
    417: http_status_pb2.ExpectationFailed,
    421: http_status_pb2.MisdirectedRequest,
    422: http_status_pb2.UnprocessableEntity,
    423: http_status_pb2.Locked,
    424: http_status_pb2.FailedDependency,
    426: http_status_pb2.UpgradeRequired,
    428: http_status_pb2.PreconditionRequired,
    429: http_status_pb2.TooManyRequests,
    431: http_status_pb2.RequestHeaderFieldsTooLarge,
    500: http_status_pb2.InternalServerError,
    501: http_status_pb2.NotImplemented,
    502: http_status_pb2.BadGateway,
    503: http_status_pb2.ServiceUnavailable,
    504: http_status_pb2.GatewayTimeout,
    505: http_status_pb2.HTTPVersionNotSupported,
    506: http_status_pb2.VariantAlsoNegotiates,
    507: http_status_pb2.InsufficientStorage,
    508: http_status_pb2.LoopDetected,
    510: http_status_pb2.NotExtended,
    511: http_status_pb2.NetworkAuthenticationRequired,
}


def map_int_to_grpc_status_code(code: int) -> http_status_pb2.StatusCode:
    return status_code_map.get(code, http_status_pb2.OK)
