from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SocketOption(_message.Message):
    __slots__ = ("description", "level", "name", "int_value", "buf_value", "state")
    class SocketState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        STATE_PREBIND: _ClassVar[SocketOption.SocketState]
        STATE_BOUND: _ClassVar[SocketOption.SocketState]
        STATE_LISTENING: _ClassVar[SocketOption.SocketState]
    STATE_PREBIND: SocketOption.SocketState
    STATE_BOUND: SocketOption.SocketState
    STATE_LISTENING: SocketOption.SocketState
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    INT_VALUE_FIELD_NUMBER: _ClassVar[int]
    BUF_VALUE_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    description: str
    level: int
    name: int
    int_value: int
    buf_value: bytes
    state: SocketOption.SocketState
    def __init__(self, description: _Optional[str] = ..., level: _Optional[int] = ..., name: _Optional[int] = ..., int_value: _Optional[int] = ..., buf_value: _Optional[bytes] = ..., state: _Optional[_Union[SocketOption.SocketState, str]] = ...) -> None: ...

class SocketOptionsOverride(_message.Message):
    __slots__ = ("socket_options",)
    SOCKET_OPTIONS_FIELD_NUMBER: _ClassVar[int]
    socket_options: _containers.RepeatedCompositeFieldContainer[SocketOption]
    def __init__(self, socket_options: _Optional[_Iterable[_Union[SocketOption, _Mapping]]] = ...) -> None: ...
