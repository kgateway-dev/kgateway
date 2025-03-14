from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class PackageVersionStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    UNKNOWN: _ClassVar[PackageVersionStatus]
    FROZEN: _ClassVar[PackageVersionStatus]
    ACTIVE: _ClassVar[PackageVersionStatus]
    NEXT_MAJOR_VERSION_CANDIDATE: _ClassVar[PackageVersionStatus]
UNKNOWN: PackageVersionStatus
FROZEN: PackageVersionStatus
ACTIVE: PackageVersionStatus
NEXT_MAJOR_VERSION_CANDIDATE: PackageVersionStatus
FILE_STATUS_FIELD_NUMBER: _ClassVar[int]
file_status: _descriptor.FieldDescriptor
MESSAGE_STATUS_FIELD_NUMBER: _ClassVar[int]
message_status: _descriptor.FieldDescriptor
FIELD_STATUS_FIELD_NUMBER: _ClassVar[int]
field_status: _descriptor.FieldDescriptor

class FileStatusAnnotation(_message.Message):
    __slots__ = ("work_in_progress",)
    WORK_IN_PROGRESS_FIELD_NUMBER: _ClassVar[int]
    work_in_progress: bool
    def __init__(self, work_in_progress: bool = ...) -> None: ...

class MessageStatusAnnotation(_message.Message):
    __slots__ = ("work_in_progress",)
    WORK_IN_PROGRESS_FIELD_NUMBER: _ClassVar[int]
    work_in_progress: bool
    def __init__(self, work_in_progress: bool = ...) -> None: ...

class FieldStatusAnnotation(_message.Message):
    __slots__ = ("work_in_progress",)
    WORK_IN_PROGRESS_FIELD_NUMBER: _ClassVar[int]
    work_in_progress: bool
    def __init__(self, work_in_progress: bool = ...) -> None: ...

class StatusAnnotation(_message.Message):
    __slots__ = ("work_in_progress", "package_version_status")
    WORK_IN_PROGRESS_FIELD_NUMBER: _ClassVar[int]
    PACKAGE_VERSION_STATUS_FIELD_NUMBER: _ClassVar[int]
    work_in_progress: bool
    package_version_status: PackageVersionStatus
    def __init__(self, work_in_progress: bool = ..., package_version_status: _Optional[_Union[PackageVersionStatus, str]] = ...) -> None: ...
