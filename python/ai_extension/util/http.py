from api.envoy.config.core.v3 import base_pb2 as base_pb2
from email.message import Message
from util.proto import get_http_header


def parse_content_type(content_type_value: str):
    """
    parse_content_type return the content type part and the charset part of the content-type string
    http content-type is using the email mimetype format defined in https://ietf.org/rfc/rfc2045.html.
    """
    msg = Message()
    msg["Content-Type"] = content_type_value

    # we encode/decode with utf-8 in many places but technically, the encoding to use
    # should be the encoding indicated in the content-type header (even it seems to be
    # always UTF-8). So, returning the charset here and store in handler in case we need it
    return msg.get_content_type(), msg.get_content_charset("")


def get_content_type(headers: base_pb2.HeaderMap) -> str:
    content_type = get_http_header(headers, "content-type")
    content_type, _ = parse_content_type(content_type)
    return content_type
