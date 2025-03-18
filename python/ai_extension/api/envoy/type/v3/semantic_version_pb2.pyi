from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class SemanticVersion(_message.Message):
    __slots__ = ("major_number", "minor_number", "patch")
    MAJOR_NUMBER_FIELD_NUMBER: _ClassVar[int]
    MINOR_NUMBER_FIELD_NUMBER: _ClassVar[int]
    PATCH_FIELD_NUMBER: _ClassVar[int]
    major_number: int
    minor_number: int
    patch: int
    def __init__(self, major_number: _Optional[int] = ..., minor_number: _Optional[int] = ..., patch: _Optional[int] = ...) -> None: ...
