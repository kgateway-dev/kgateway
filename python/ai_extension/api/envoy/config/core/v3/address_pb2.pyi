from envoy.config.core.v3 import extension_pb2 as _extension_pb2
from envoy.config.core.v3 import socket_option_pb2 as _socket_option_pb2
from google.protobuf import wrappers_pb2 as _wrappers_pb2
from envoy.annotations import deprecation_pb2 as _deprecation_pb2
from udpa.annotations import status_pb2 as _status_pb2
from udpa.annotations import versioning_pb2 as _versioning_pb2
from validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Pipe(_message.Message):
    __slots__ = ("path", "mode")
    PATH_FIELD_NUMBER: _ClassVar[int]
    MODE_FIELD_NUMBER: _ClassVar[int]
    path: str
    mode: int
    def __init__(self, path: _Optional[str] = ..., mode: _Optional[int] = ...) -> None: ...

class EnvoyInternalAddress(_message.Message):
    __slots__ = ("server_listener_name", "endpoint_id")
    SERVER_LISTENER_NAME_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_ID_FIELD_NUMBER: _ClassVar[int]
    server_listener_name: str
    endpoint_id: str
    def __init__(self, server_listener_name: _Optional[str] = ..., endpoint_id: _Optional[str] = ...) -> None: ...

class SocketAddress(_message.Message):
    __slots__ = ("protocol", "address", "port_value", "named_port", "resolver_name", "ipv4_compat")
    class Protocol(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
        __slots__ = ()
        TCP: _ClassVar[SocketAddress.Protocol]
        UDP: _ClassVar[SocketAddress.Protocol]
    TCP: SocketAddress.Protocol
    UDP: SocketAddress.Protocol
    PROTOCOL_FIELD_NUMBER: _ClassVar[int]
    ADDRESS_FIELD_NUMBER: _ClassVar[int]
    PORT_VALUE_FIELD_NUMBER: _ClassVar[int]
    NAMED_PORT_FIELD_NUMBER: _ClassVar[int]
    RESOLVER_NAME_FIELD_NUMBER: _ClassVar[int]
    IPV4_COMPAT_FIELD_NUMBER: _ClassVar[int]
    protocol: SocketAddress.Protocol
    address: str
    port_value: int
    named_port: str
    resolver_name: str
    ipv4_compat: bool
    def __init__(self, protocol: _Optional[_Union[SocketAddress.Protocol, str]] = ..., address: _Optional[str] = ..., port_value: _Optional[int] = ..., named_port: _Optional[str] = ..., resolver_name: _Optional[str] = ..., ipv4_compat: bool = ...) -> None: ...

class TcpKeepalive(_message.Message):
    __slots__ = ("keepalive_probes", "keepalive_time", "keepalive_interval")
    KEEPALIVE_PROBES_FIELD_NUMBER: _ClassVar[int]
    KEEPALIVE_TIME_FIELD_NUMBER: _ClassVar[int]
    KEEPALIVE_INTERVAL_FIELD_NUMBER: _ClassVar[int]
    keepalive_probes: _wrappers_pb2.UInt32Value
    keepalive_time: _wrappers_pb2.UInt32Value
    keepalive_interval: _wrappers_pb2.UInt32Value
    def __init__(self, keepalive_probes: _Optional[_Union[_wrappers_pb2.UInt32Value, _Mapping]] = ..., keepalive_time: _Optional[_Union[_wrappers_pb2.UInt32Value, _Mapping]] = ..., keepalive_interval: _Optional[_Union[_wrappers_pb2.UInt32Value, _Mapping]] = ...) -> None: ...

class ExtraSourceAddress(_message.Message):
    __slots__ = ("address", "socket_options")
    ADDRESS_FIELD_NUMBER: _ClassVar[int]
    SOCKET_OPTIONS_FIELD_NUMBER: _ClassVar[int]
    address: SocketAddress
    socket_options: _socket_option_pb2.SocketOptionsOverride
    def __init__(self, address: _Optional[_Union[SocketAddress, _Mapping]] = ..., socket_options: _Optional[_Union[_socket_option_pb2.SocketOptionsOverride, _Mapping]] = ...) -> None: ...

class BindConfig(_message.Message):
    __slots__ = ("source_address", "freebind", "socket_options", "extra_source_addresses", "additional_source_addresses", "local_address_selector")
    SOURCE_ADDRESS_FIELD_NUMBER: _ClassVar[int]
    FREEBIND_FIELD_NUMBER: _ClassVar[int]
    SOCKET_OPTIONS_FIELD_NUMBER: _ClassVar[int]
    EXTRA_SOURCE_ADDRESSES_FIELD_NUMBER: _ClassVar[int]
    ADDITIONAL_SOURCE_ADDRESSES_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ADDRESS_SELECTOR_FIELD_NUMBER: _ClassVar[int]
    source_address: SocketAddress
    freebind: _wrappers_pb2.BoolValue
    socket_options: _containers.RepeatedCompositeFieldContainer[_socket_option_pb2.SocketOption]
    extra_source_addresses: _containers.RepeatedCompositeFieldContainer[ExtraSourceAddress]
    additional_source_addresses: _containers.RepeatedCompositeFieldContainer[SocketAddress]
    local_address_selector: _extension_pb2.TypedExtensionConfig
    def __init__(self, source_address: _Optional[_Union[SocketAddress, _Mapping]] = ..., freebind: _Optional[_Union[_wrappers_pb2.BoolValue, _Mapping]] = ..., socket_options: _Optional[_Iterable[_Union[_socket_option_pb2.SocketOption, _Mapping]]] = ..., extra_source_addresses: _Optional[_Iterable[_Union[ExtraSourceAddress, _Mapping]]] = ..., additional_source_addresses: _Optional[_Iterable[_Union[SocketAddress, _Mapping]]] = ..., local_address_selector: _Optional[_Union[_extension_pb2.TypedExtensionConfig, _Mapping]] = ...) -> None: ...

class Address(_message.Message):
    __slots__ = ("socket_address", "pipe", "envoy_internal_address")
    SOCKET_ADDRESS_FIELD_NUMBER: _ClassVar[int]
    PIPE_FIELD_NUMBER: _ClassVar[int]
    ENVOY_INTERNAL_ADDRESS_FIELD_NUMBER: _ClassVar[int]
    socket_address: SocketAddress
    pipe: Pipe
    envoy_internal_address: EnvoyInternalAddress
    def __init__(self, socket_address: _Optional[_Union[SocketAddress, _Mapping]] = ..., pipe: _Optional[_Union[Pipe, _Mapping]] = ..., envoy_internal_address: _Optional[_Union[EnvoyInternalAddress, _Mapping]] = ...) -> None: ...

class CidrRange(_message.Message):
    __slots__ = ("address_prefix", "prefix_len")
    ADDRESS_PREFIX_FIELD_NUMBER: _ClassVar[int]
    PREFIX_LEN_FIELD_NUMBER: _ClassVar[int]
    address_prefix: str
    prefix_len: _wrappers_pb2.UInt32Value
    def __init__(self, address_prefix: _Optional[str] = ..., prefix_len: _Optional[_Union[_wrappers_pb2.UInt32Value, _Mapping]] = ...) -> None: ...
