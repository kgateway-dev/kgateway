from udpa.annotations import status_pb2 as _status_pb2
from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor
SECURITY_FIELD_NUMBER: _ClassVar[int]
security: _descriptor.FieldDescriptor

class FieldSecurityAnnotation(_message.Message):
    __slots__ = ("configure_for_untrusted_downstream", "configure_for_untrusted_upstream")
    CONFIGURE_FOR_UNTRUSTED_DOWNSTREAM_FIELD_NUMBER: _ClassVar[int]
    CONFIGURE_FOR_UNTRUSTED_UPSTREAM_FIELD_NUMBER: _ClassVar[int]
    configure_for_untrusted_downstream: bool
    configure_for_untrusted_upstream: bool
    def __init__(self, configure_for_untrusted_downstream: bool = ..., configure_for_untrusted_upstream: bool = ...) -> None: ...
