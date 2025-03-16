from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import duration_pb2 as _duration_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class KnownRegex(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    UNKNOWN: _ClassVar[KnownRegex]
    HTTP_HEADER_NAME: _ClassVar[KnownRegex]
    HTTP_HEADER_VALUE: _ClassVar[KnownRegex]
UNKNOWN: KnownRegex
HTTP_HEADER_NAME: KnownRegex
HTTP_HEADER_VALUE: KnownRegex
DISABLED_FIELD_NUMBER: _ClassVar[int]
disabled: _descriptor.FieldDescriptor
IGNORED_FIELD_NUMBER: _ClassVar[int]
ignored: _descriptor.FieldDescriptor
REQUIRED_FIELD_NUMBER: _ClassVar[int]
required: _descriptor.FieldDescriptor
RULES_FIELD_NUMBER: _ClassVar[int]
rules: _descriptor.FieldDescriptor

class FieldRules(_message.Message):
    __slots__ = ("message", "float", "double", "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64", "bool", "string", "bytes", "enum", "repeated", "map", "any", "duration", "timestamp")
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    FLOAT_FIELD_NUMBER: _ClassVar[int]
    DOUBLE_FIELD_NUMBER: _ClassVar[int]
    INT32_FIELD_NUMBER: _ClassVar[int]
    INT64_FIELD_NUMBER: _ClassVar[int]
    UINT32_FIELD_NUMBER: _ClassVar[int]
    UINT64_FIELD_NUMBER: _ClassVar[int]
    SINT32_FIELD_NUMBER: _ClassVar[int]
    SINT64_FIELD_NUMBER: _ClassVar[int]
    FIXED32_FIELD_NUMBER: _ClassVar[int]
    FIXED64_FIELD_NUMBER: _ClassVar[int]
    SFIXED32_FIELD_NUMBER: _ClassVar[int]
    SFIXED64_FIELD_NUMBER: _ClassVar[int]
    BOOL_FIELD_NUMBER: _ClassVar[int]
    STRING_FIELD_NUMBER: _ClassVar[int]
    BYTES_FIELD_NUMBER: _ClassVar[int]
    ENUM_FIELD_NUMBER: _ClassVar[int]
    REPEATED_FIELD_NUMBER: _ClassVar[int]
    MAP_FIELD_NUMBER: _ClassVar[int]
    ANY_FIELD_NUMBER: _ClassVar[int]
    DURATION_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    message: MessageRules
    float: FloatRules
    double: DoubleRules
    int32: Int32Rules
    int64: Int64Rules
    uint32: UInt32Rules
    uint64: UInt64Rules
    sint32: SInt32Rules
    sint64: SInt64Rules
    fixed32: Fixed32Rules
    fixed64: Fixed64Rules
    sfixed32: SFixed32Rules
    sfixed64: SFixed64Rules
    bool: BoolRules
    string: StringRules
    bytes: BytesRules
    enum: EnumRules
    repeated: RepeatedRules
    map: MapRules
    any: AnyRules
    duration: DurationRules
    timestamp: TimestampRules
    def __init__(self, message: _Optional[_Union[MessageRules, _Mapping]] = ..., float: _Optional[_Union[FloatRules, _Mapping]] = ..., double: _Optional[_Union[DoubleRules, _Mapping]] = ..., int32: _Optional[_Union[Int32Rules, _Mapping]] = ..., int64: _Optional[_Union[Int64Rules, _Mapping]] = ..., uint32: _Optional[_Union[UInt32Rules, _Mapping]] = ..., uint64: _Optional[_Union[UInt64Rules, _Mapping]] = ..., sint32: _Optional[_Union[SInt32Rules, _Mapping]] = ..., sint64: _Optional[_Union[SInt64Rules, _Mapping]] = ..., fixed32: _Optional[_Union[Fixed32Rules, _Mapping]] = ..., fixed64: _Optional[_Union[Fixed64Rules, _Mapping]] = ..., sfixed32: _Optional[_Union[SFixed32Rules, _Mapping]] = ..., sfixed64: _Optional[_Union[SFixed64Rules, _Mapping]] = ..., bool: _Optional[_Union[BoolRules, _Mapping]] = ..., string: _Optional[_Union[StringRules, _Mapping]] = ..., bytes: _Optional[_Union[BytesRules, _Mapping]] = ..., enum: _Optional[_Union[EnumRules, _Mapping]] = ..., repeated: _Optional[_Union[RepeatedRules, _Mapping]] = ..., map: _Optional[_Union[MapRules, _Mapping]] = ..., any: _Optional[_Union[AnyRules, _Mapping]] = ..., duration: _Optional[_Union[DurationRules, _Mapping]] = ..., timestamp: _Optional[_Union[TimestampRules, _Mapping]] = ...) -> None: ...

class FloatRules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: float
    lt: float
    lte: float
    gt: float
    gte: float
    not_in: _containers.RepeatedScalarFieldContainer[float]
    ignore_empty: bool
    def __init__(self, const: _Optional[float] = ..., lt: _Optional[float] = ..., lte: _Optional[float] = ..., gt: _Optional[float] = ..., gte: _Optional[float] = ..., not_in: _Optional[_Iterable[float]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class DoubleRules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: float
    lt: float
    lte: float
    gt: float
    gte: float
    not_in: _containers.RepeatedScalarFieldContainer[float]
    ignore_empty: bool
    def __init__(self, const: _Optional[float] = ..., lt: _Optional[float] = ..., lte: _Optional[float] = ..., gt: _Optional[float] = ..., gte: _Optional[float] = ..., not_in: _Optional[_Iterable[float]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class Int32Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class Int64Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class UInt32Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class UInt64Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class SInt32Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class SInt64Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class Fixed32Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class Fixed64Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class SFixed32Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class SFixed64Rules(_message.Message):
    __slots__ = ("const", "lt", "lte", "gt", "gte", "not_in", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: int
    lt: int
    lte: int
    gt: int
    gte: int
    not_in: _containers.RepeatedScalarFieldContainer[int]
    ignore_empty: bool
    def __init__(self, const: _Optional[int] = ..., lt: _Optional[int] = ..., lte: _Optional[int] = ..., gt: _Optional[int] = ..., gte: _Optional[int] = ..., not_in: _Optional[_Iterable[int]] = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class BoolRules(_message.Message):
    __slots__ = ("const",)
    CONST_FIELD_NUMBER: _ClassVar[int]
    const: bool
    def __init__(self, const: bool = ...) -> None: ...

class StringRules(_message.Message):
    __slots__ = ("const", "len", "min_len", "max_len", "len_bytes", "min_bytes", "max_bytes", "pattern", "prefix", "suffix", "contains", "not_contains", "not_in", "email", "hostname", "ip", "ipv4", "ipv6", "uri", "uri_ref", "address", "uuid", "well_known_regex", "strict", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LEN_FIELD_NUMBER: _ClassVar[int]
    MIN_LEN_FIELD_NUMBER: _ClassVar[int]
    MAX_LEN_FIELD_NUMBER: _ClassVar[int]
    LEN_BYTES_FIELD_NUMBER: _ClassVar[int]
    MIN_BYTES_FIELD_NUMBER: _ClassVar[int]
    MAX_BYTES_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    PREFIX_FIELD_NUMBER: _ClassVar[int]
    SUFFIX_FIELD_NUMBER: _ClassVar[int]
    CONTAINS_FIELD_NUMBER: _ClassVar[int]
    NOT_CONTAINS_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    EMAIL_FIELD_NUMBER: _ClassVar[int]
    HOSTNAME_FIELD_NUMBER: _ClassVar[int]
    IP_FIELD_NUMBER: _ClassVar[int]
    IPV4_FIELD_NUMBER: _ClassVar[int]
    IPV6_FIELD_NUMBER: _ClassVar[int]
    URI_FIELD_NUMBER: _ClassVar[int]
    URI_REF_FIELD_NUMBER: _ClassVar[int]
    ADDRESS_FIELD_NUMBER: _ClassVar[int]
    UUID_FIELD_NUMBER: _ClassVar[int]
    WELL_KNOWN_REGEX_FIELD_NUMBER: _ClassVar[int]
    STRICT_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: str
    len: int
    min_len: int
    max_len: int
    len_bytes: int
    min_bytes: int
    max_bytes: int
    pattern: str
    prefix: str
    suffix: str
    contains: str
    not_contains: str
    not_in: _containers.RepeatedScalarFieldContainer[str]
    email: bool
    hostname: bool
    ip: bool
    ipv4: bool
    ipv6: bool
    uri: bool
    uri_ref: bool
    address: bool
    uuid: bool
    well_known_regex: KnownRegex
    strict: bool
    ignore_empty: bool
    def __init__(self, const: _Optional[str] = ..., len: _Optional[int] = ..., min_len: _Optional[int] = ..., max_len: _Optional[int] = ..., len_bytes: _Optional[int] = ..., min_bytes: _Optional[int] = ..., max_bytes: _Optional[int] = ..., pattern: _Optional[str] = ..., prefix: _Optional[str] = ..., suffix: _Optional[str] = ..., contains: _Optional[str] = ..., not_contains: _Optional[str] = ..., not_in: _Optional[_Iterable[str]] = ..., email: bool = ..., hostname: bool = ..., ip: bool = ..., ipv4: bool = ..., ipv6: bool = ..., uri: bool = ..., uri_ref: bool = ..., address: bool = ..., uuid: bool = ..., well_known_regex: _Optional[_Union[KnownRegex, str]] = ..., strict: bool = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class BytesRules(_message.Message):
    __slots__ = ("const", "len", "min_len", "max_len", "pattern", "prefix", "suffix", "contains", "not_in", "ip", "ipv4", "ipv6", "ignore_empty")
    CONST_FIELD_NUMBER: _ClassVar[int]
    LEN_FIELD_NUMBER: _ClassVar[int]
    MIN_LEN_FIELD_NUMBER: _ClassVar[int]
    MAX_LEN_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    PREFIX_FIELD_NUMBER: _ClassVar[int]
    SUFFIX_FIELD_NUMBER: _ClassVar[int]
    CONTAINS_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    IP_FIELD_NUMBER: _ClassVar[int]
    IPV4_FIELD_NUMBER: _ClassVar[int]
    IPV6_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    const: bytes
    len: int
    min_len: int
    max_len: int
    pattern: str
    prefix: bytes
    suffix: bytes
    contains: bytes
    not_in: _containers.RepeatedScalarFieldContainer[bytes]
    ip: bool
    ipv4: bool
    ipv6: bool
    ignore_empty: bool
    def __init__(self, const: _Optional[bytes] = ..., len: _Optional[int] = ..., min_len: _Optional[int] = ..., max_len: _Optional[int] = ..., pattern: _Optional[str] = ..., prefix: _Optional[bytes] = ..., suffix: _Optional[bytes] = ..., contains: _Optional[bytes] = ..., not_in: _Optional[_Iterable[bytes]] = ..., ip: bool = ..., ipv4: bool = ..., ipv6: bool = ..., ignore_empty: bool = ..., **kwargs) -> None: ...

class EnumRules(_message.Message):
    __slots__ = ("const", "defined_only", "not_in")
    CONST_FIELD_NUMBER: _ClassVar[int]
    DEFINED_ONLY_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    const: int
    defined_only: bool
    not_in: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, const: _Optional[int] = ..., defined_only: bool = ..., not_in: _Optional[_Iterable[int]] = ..., **kwargs) -> None: ...

class MessageRules(_message.Message):
    __slots__ = ("skip", "required")
    SKIP_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    skip: bool
    required: bool
    def __init__(self, skip: bool = ..., required: bool = ...) -> None: ...

class RepeatedRules(_message.Message):
    __slots__ = ("min_items", "max_items", "unique", "items", "ignore_empty")
    MIN_ITEMS_FIELD_NUMBER: _ClassVar[int]
    MAX_ITEMS_FIELD_NUMBER: _ClassVar[int]
    UNIQUE_FIELD_NUMBER: _ClassVar[int]
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    min_items: int
    max_items: int
    unique: bool
    items: FieldRules
    ignore_empty: bool
    def __init__(self, min_items: _Optional[int] = ..., max_items: _Optional[int] = ..., unique: bool = ..., items: _Optional[_Union[FieldRules, _Mapping]] = ..., ignore_empty: bool = ...) -> None: ...

class MapRules(_message.Message):
    __slots__ = ("min_pairs", "max_pairs", "no_sparse", "keys", "values", "ignore_empty")
    MIN_PAIRS_FIELD_NUMBER: _ClassVar[int]
    MAX_PAIRS_FIELD_NUMBER: _ClassVar[int]
    NO_SPARSE_FIELD_NUMBER: _ClassVar[int]
    KEYS_FIELD_NUMBER: _ClassVar[int]
    VALUES_FIELD_NUMBER: _ClassVar[int]
    IGNORE_EMPTY_FIELD_NUMBER: _ClassVar[int]
    min_pairs: int
    max_pairs: int
    no_sparse: bool
    keys: FieldRules
    values: FieldRules
    ignore_empty: bool
    def __init__(self, min_pairs: _Optional[int] = ..., max_pairs: _Optional[int] = ..., no_sparse: bool = ..., keys: _Optional[_Union[FieldRules, _Mapping]] = ..., values: _Optional[_Union[FieldRules, _Mapping]] = ..., ignore_empty: bool = ...) -> None: ...

class AnyRules(_message.Message):
    __slots__ = ("required", "not_in")
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    required: bool
    not_in: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, required: bool = ..., not_in: _Optional[_Iterable[str]] = ..., **kwargs) -> None: ...

class DurationRules(_message.Message):
    __slots__ = ("required", "const", "lt", "lte", "gt", "gte", "not_in")
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    IN_FIELD_NUMBER: _ClassVar[int]
    NOT_IN_FIELD_NUMBER: _ClassVar[int]
    required: bool
    const: _duration_pb2.Duration
    lt: _duration_pb2.Duration
    lte: _duration_pb2.Duration
    gt: _duration_pb2.Duration
    gte: _duration_pb2.Duration
    not_in: _containers.RepeatedCompositeFieldContainer[_duration_pb2.Duration]
    def __init__(self, required: bool = ..., const: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., lt: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., lte: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., gt: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., gte: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ..., not_in: _Optional[_Iterable[_Union[_duration_pb2.Duration, _Mapping]]] = ..., **kwargs) -> None: ...

class TimestampRules(_message.Message):
    __slots__ = ("required", "const", "lt", "lte", "gt", "gte", "lt_now", "gt_now", "within")
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    CONST_FIELD_NUMBER: _ClassVar[int]
    LT_FIELD_NUMBER: _ClassVar[int]
    LTE_FIELD_NUMBER: _ClassVar[int]
    GT_FIELD_NUMBER: _ClassVar[int]
    GTE_FIELD_NUMBER: _ClassVar[int]
    LT_NOW_FIELD_NUMBER: _ClassVar[int]
    GT_NOW_FIELD_NUMBER: _ClassVar[int]
    WITHIN_FIELD_NUMBER: _ClassVar[int]
    required: bool
    const: _timestamp_pb2.Timestamp
    lt: _timestamp_pb2.Timestamp
    lte: _timestamp_pb2.Timestamp
    gt: _timestamp_pb2.Timestamp
    gte: _timestamp_pb2.Timestamp
    lt_now: bool
    gt_now: bool
    within: _duration_pb2.Duration
    def __init__(self, required: bool = ..., const: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., lt: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., lte: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., gt: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., gte: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., lt_now: bool = ..., gt_now: bool = ..., within: _Optional[_Union[_duration_pb2.Duration, _Mapping]] = ...) -> None: ...
