from telemetry.stats import CustomLabel
from google.protobuf import struct_pb2 as struct_pb2
from google.protobuf.json_format import ParseDict


class TestStats:
    def test_standard_config(self):
        config: dict = {
            "name": "test",
            "metadataKey": "principal:team",
        }

        custom_label = CustomLabel(**config)
        assert custom_label is not None
        struct: dict = {
            "principal": {
                "team": "test",
                "org": "test",
            }
        }
        proto_struct = struct_pb2.Struct()
        ParseDict(struct, proto_struct)
        print(proto_struct)
        assert custom_label.get_field(proto_struct) == "test"

    def test_custom_delimiter(self):
        config: dict = {
            "name": "test",
            "metadataKey": "principal.team",
            "keyDelimiter": ".",
        }

        custom_label = CustomLabel(**config)
        assert custom_label is not None
        struct: dict = {
            "principal": {
                "team": "test",
                "org": "test",
            }
        }
        proto_struct = struct_pb2.Struct()
        ParseDict(struct, proto_struct)
        assert custom_label.get_field(proto_struct) == "test"
