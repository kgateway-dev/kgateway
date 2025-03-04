from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class StatusCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    Empty: _ClassVar[StatusCode]
    Continue: _ClassVar[StatusCode]
    OK: _ClassVar[StatusCode]
    Created: _ClassVar[StatusCode]
    Accepted: _ClassVar[StatusCode]
    NonAuthoritativeInformation: _ClassVar[StatusCode]
    NoContent: _ClassVar[StatusCode]
    ResetContent: _ClassVar[StatusCode]
    PartialContent: _ClassVar[StatusCode]
    MultiStatus: _ClassVar[StatusCode]
    AlreadyReported: _ClassVar[StatusCode]
    IMUsed: _ClassVar[StatusCode]
    MultipleChoices: _ClassVar[StatusCode]
    MovedPermanently: _ClassVar[StatusCode]
    Found: _ClassVar[StatusCode]
    SeeOther: _ClassVar[StatusCode]
    NotModified: _ClassVar[StatusCode]
    UseProxy: _ClassVar[StatusCode]
    TemporaryRedirect: _ClassVar[StatusCode]
    PermanentRedirect: _ClassVar[StatusCode]
    BadRequest: _ClassVar[StatusCode]
    Unauthorized: _ClassVar[StatusCode]
    PaymentRequired: _ClassVar[StatusCode]
    Forbidden: _ClassVar[StatusCode]
    NotFound: _ClassVar[StatusCode]
    MethodNotAllowed: _ClassVar[StatusCode]
    NotAcceptable: _ClassVar[StatusCode]
    ProxyAuthenticationRequired: _ClassVar[StatusCode]
    RequestTimeout: _ClassVar[StatusCode]
    Conflict: _ClassVar[StatusCode]
    Gone: _ClassVar[StatusCode]
    LengthRequired: _ClassVar[StatusCode]
    PreconditionFailed: _ClassVar[StatusCode]
    PayloadTooLarge: _ClassVar[StatusCode]
    URITooLong: _ClassVar[StatusCode]
    UnsupportedMediaType: _ClassVar[StatusCode]
    RangeNotSatisfiable: _ClassVar[StatusCode]
    ExpectationFailed: _ClassVar[StatusCode]
    MisdirectedRequest: _ClassVar[StatusCode]
    UnprocessableEntity: _ClassVar[StatusCode]
    Locked: _ClassVar[StatusCode]
    FailedDependency: _ClassVar[StatusCode]
    UpgradeRequired: _ClassVar[StatusCode]
    PreconditionRequired: _ClassVar[StatusCode]
    TooManyRequests: _ClassVar[StatusCode]
    RequestHeaderFieldsTooLarge: _ClassVar[StatusCode]
    InternalServerError: _ClassVar[StatusCode]
    NotImplemented: _ClassVar[StatusCode]
    BadGateway: _ClassVar[StatusCode]
    ServiceUnavailable: _ClassVar[StatusCode]
    GatewayTimeout: _ClassVar[StatusCode]
    HTTPVersionNotSupported: _ClassVar[StatusCode]
    VariantAlsoNegotiates: _ClassVar[StatusCode]
    InsufficientStorage: _ClassVar[StatusCode]
    LoopDetected: _ClassVar[StatusCode]
    NotExtended: _ClassVar[StatusCode]
    NetworkAuthenticationRequired: _ClassVar[StatusCode]
Empty: StatusCode
Continue: StatusCode
OK: StatusCode
Created: StatusCode
Accepted: StatusCode
NonAuthoritativeInformation: StatusCode
NoContent: StatusCode
ResetContent: StatusCode
PartialContent: StatusCode
MultiStatus: StatusCode
AlreadyReported: StatusCode
IMUsed: StatusCode
MultipleChoices: StatusCode
MovedPermanently: StatusCode
Found: StatusCode
SeeOther: StatusCode
NotModified: StatusCode
UseProxy: StatusCode
TemporaryRedirect: StatusCode
PermanentRedirect: StatusCode
BadRequest: StatusCode
Unauthorized: StatusCode
PaymentRequired: StatusCode
Forbidden: StatusCode
NotFound: StatusCode
MethodNotAllowed: StatusCode
NotAcceptable: StatusCode
ProxyAuthenticationRequired: StatusCode
RequestTimeout: StatusCode
Conflict: StatusCode
Gone: StatusCode
LengthRequired: StatusCode
PreconditionFailed: StatusCode
PayloadTooLarge: StatusCode
URITooLong: StatusCode
UnsupportedMediaType: StatusCode
RangeNotSatisfiable: StatusCode
ExpectationFailed: StatusCode
MisdirectedRequest: StatusCode
UnprocessableEntity: StatusCode
Locked: StatusCode
FailedDependency: StatusCode
UpgradeRequired: StatusCode
PreconditionRequired: StatusCode
TooManyRequests: StatusCode
RequestHeaderFieldsTooLarge: StatusCode
InternalServerError: StatusCode
NotImplemented: StatusCode
BadGateway: StatusCode
ServiceUnavailable: StatusCode
GatewayTimeout: StatusCode
HTTPVersionNotSupported: StatusCode
VariantAlsoNegotiates: StatusCode
InsufficientStorage: StatusCode
LoopDetected: StatusCode
NotExtended: StatusCode
NetworkAuthenticationRequired: StatusCode

class HttpStatus(_message.Message):
    __slots__ = ("code",)
    CODE_FIELD_NUMBER: _ClassVar[int]
    code: StatusCode
    def __init__(self, code: _Optional[_Union[StatusCode, str]] = ...) -> None: ...
