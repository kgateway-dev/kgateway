from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Percent(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: float
    def __init__(self, value: _Optional[float] = ...) -> None: ...

class FractionalPercent(_message.Message):
    __slots__ = ("numerator", "denominator")
    class DenominatorType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        HUNDRED: _ClassVar[FractionalPercent.DenominatorType]
        TEN_THOUSAND: _ClassVar[FractionalPercent.DenominatorType]
        MILLION: _ClassVar[FractionalPercent.DenominatorType]
    HUNDRED: FractionalPercent.DenominatorType
    TEN_THOUSAND: FractionalPercent.DenominatorType
    MILLION: FractionalPercent.DenominatorType
    NUMERATOR_FIELD_NUMBER: _ClassVar[int]
    DENOMINATOR_FIELD_NUMBER: _ClassVar[int]
    numerator: int
    denominator: FractionalPercent.DenominatorType
    def __init__(self, numerator: _Optional[int] = ..., denominator: _Optional[_Union[FractionalPercent.DenominatorType, str]] = ...) -> None: ...
