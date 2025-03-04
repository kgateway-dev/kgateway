import json
import logging

from google.protobuf import struct_pb2 as struct_pb2
from .utils import read_mounted_config_map

from pydantic import BaseModel, Field

# OpenTelemetry imports
from opentelemetry.trace import Tracer, NoOpTracer
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import (
    BatchSpanProcessor,
)
from opentelemetry.sdk.resources import (
    SERVICE_NAME,
    Resource,
)

from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

logger: logging.Logger = logging.getLogger().getChild(
    "kgateway-ai-ext.external_processor"
)


class Grpc(BaseModel):
    """
    Represents a gRPC configuration for the AI Tracing config.
    """

    host: str = Field(default="")
    port: int = Field(default=0)


class Config(BaseModel):
    """
    Python representation of the AI Tracing config struct from the Gateway Parameters API.

    using pydantic.BaseModel allows for much simpler Json parsing and validation.
    """

    grpc: Grpc | None = Field(default=None)

    insecure: bool = Field(default=False)

    def tracer(self) -> Tracer:
        # Initialize tracer provider
        resource = Resource.create(attributes={SERVICE_NAME: "kgateway-ai-extension"})
        tracer_provider = TracerProvider(resource=resource)
        # pydantic.BaseModel is creating a default grpc object instead of making it None
        # even the tracing.json file doesn't exist (not configured) and we initialize the
        # Config object with an empty json. So, need the extra checks here to avoid the
        # Tracer error messages and retries when tracing is disabled.
        if self.grpc is not None and len(self.grpc.host) > 0 and self.grpc.port > 0:
            url = f"{self.grpc.host}:{self.grpc.port}"
            logger.debug(f"tracer publishing to: {url}")
            # Configure span processor and exporter
            span_processor = BatchSpanProcessor(
                OTLPSpanExporter(endpoint=url, insecure=self.insecure)
            )
            tracer_provider.add_span_processor(span_processor)
        else:
            logger.warning("No gRPC configuration found. Tracing will not be enabled.")
            tracer_provider._disabled = True

        return tracer_provider.get_tracer(__name__)

    @classmethod
    def from_file(cls, file_path: str):
        try:
            data = read_mounted_config_map(file_path)
            return cls(**data)
        except FileNotFoundError as e:
            logger.warning(f"Error: tracing file config was not found: {e}")
            return cls(grpc=Grpc())
        except json.JSONDecodeError as e:
            logger.warning(f"Error: tracing file config is not a valid JSON: {e}")
            return cls(grpc=Grpc())
        except Exception as ex:
            logger.warning(f"An error occurred while reading the file. {ex}")
            return cls(grpc=Grpc())


class OtelTracer:
    """
    A static class that hold the reference to the tracer instance
    """

    __tracer: Tracer | None = None

    @classmethod
    def get(cls) -> Tracer | NoOpTracer:
        if cls.__tracer is None:
            logger.debug(
                "OtelTracer is used before initializing. Returning NoOpTracer!"
            )
            return NoOpTracer()
        return cls.__tracer

    @classmethod
    def init(cls, tracer: Tracer):
        if cls.__tracer is None:
            cls.__tracer = tracer
