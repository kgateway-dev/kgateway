from google.protobuf import duration_pb2 as _duration_pb2
from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class BackoffStrategy(_message.Message):
    __slots__ = ("base_interval", "max_interval")
    BASE_INTERVAL_FIELD_NUMBER: _ClassVar[int]
    MAX_INTERVAL_FIELD_NUMBER: _ClassVar[int]
    base_interval: _duration_pb2.Duration
    max_interval: _duration_pb2.Duration
    def __init__(self, base_interval: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., max_interval: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...
