from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor
MESSAGE_MIGRATE_FIELD_NUMBER: _ClassVar[int]
message_migrate: _descriptor.FieldDescriptor
FIELD_MIGRATE_FIELD_NUMBER: _ClassVar[int]
field_migrate: _descriptor.FieldDescriptor
ENUM_MIGRATE_FIELD_NUMBER: _ClassVar[int]
enum_migrate: _descriptor.FieldDescriptor
ENUM_VALUE_MIGRATE_FIELD_NUMBER: _ClassVar[int]
enum_value_migrate: _descriptor.FieldDescriptor
FILE_MIGRATE_FIELD_NUMBER: _ClassVar[int]
file_migrate: _descriptor.FieldDescriptor

class MigrateAnnotation(_message.Message):
    __slots__ = ("rename",)
    RENAME_FIELD_NUMBER: _ClassVar[int]
    rename: str
    def __init__(self, rename: _Optional[str] = ...) -> None: ...

class FieldMigrateAnnotation(_message.Message):
    __slots__ = ("rename", "oneof_promotion")
    RENAME_FIELD_NUMBER: _ClassVar[int]
    ONEOF_PROMOTION_FIELD_NUMBER: _ClassVar[int]
    rename: str
    oneof_promotion: str
    def __init__(self, rename: _Optional[str] = ..., oneof_promotion: _Optional[str] = ...) -> None: ...

class FileMigrateAnnotation(_message.Message):
    __slots__ = ("move_to_package",)
    MOVE_TO_PACKAGE_FIELD_NUMBER: _ClassVar[int]
    move_to_package: str
    def __init__(self, move_to_package: _Optional[str] = ...) -> None: ...
