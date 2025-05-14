from udpa.annotations import status_pb2 as _status_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ProcessingMode(_message.Message):
    __slots__ = ("request_header_mode", "response_header_mode", "request_body_mode", "response_body_mode", "request_trailer_mode", "response_trailer_mode")
    class HeaderSendMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        DEFAULT: _ClassVar[ProcessingMode.HeaderSendMode]
        SEND: _ClassVar[ProcessingMode.HeaderSendMode]
        SKIP: _ClassVar[ProcessingMode.HeaderSendMode]
    DEFAULT: ProcessingMode.HeaderSendMode
    SEND: ProcessingMode.HeaderSendMode
    SKIP: ProcessingMode.HeaderSendMode
    class BodySendMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        NONE: _ClassVar[ProcessingMode.BodySendMode]
        STREAMED: _ClassVar[ProcessingMode.BodySendMode]
        BUFFERED: _ClassVar[ProcessingMode.BodySendMode]
        BUFFERED_PARTIAL: _ClassVar[ProcessingMode.BodySendMode]
    NONE: ProcessingMode.BodySendMode
    STREAMED: ProcessingMode.BodySendMode
    BUFFERED: ProcessingMode.BodySendMode
    BUFFERED_PARTIAL: ProcessingMode.BodySendMode
    REQUEST_HEADER_MODE_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_HEADER_MODE_FIELD_NUMBER: _ClassVar[int]
    REQUEST_BODY_MODE_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_BODY_MODE_FIELD_NUMBER: _ClassVar[int]
    REQUEST_TRAILER_MODE_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_TRAILER_MODE_FIELD_NUMBER: _ClassVar[int]
    request_header_mode: ProcessingMode.HeaderSendMode
    response_header_mode: ProcessingMode.HeaderSendMode
    request_body_mode: ProcessingMode.BodySendMode
    response_body_mode: ProcessingMode.BodySendMode
    request_trailer_mode: ProcessingMode.HeaderSendMode
    response_trailer_mode: ProcessingMode.HeaderSendMode
    def __init__(self, request_header_mode: _Optional[_Union[ProcessingMode.HeaderSendMode, str]] = ..., response_header_mode: _Optional[_Union[ProcessingMode.HeaderSendMode, str]] = ..., request_body_mode: _Optional[_Union[ProcessingMode.BodySendMode, str]] = ..., response_body_mode: _Optional[_Union[ProcessingMode.BodySendMode, str]] = ..., request_trailer_mode: _Optional[_Union[ProcessingMode.HeaderSendMode, str]] = ..., response_trailer_mode: _Optional[_Union[ProcessingMode.HeaderSendMode, str]] = ...) -> None: ...
