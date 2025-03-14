from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor
VERSIONING_FIELD_NUMBER: _ClassVar[int]
versioning: _descriptor.FieldDescriptor

class VersioningAnnotation(_message.Message):
    __slots__ = ("previous_message_type",)
    PREVIOUS_MESSAGE_TYPE_FIELD_NUMBER: _ClassVar[int]
    previous_message_type: str
    def __init__(self, previous_message_type: _Optional[str] = ...) -> None: ...
