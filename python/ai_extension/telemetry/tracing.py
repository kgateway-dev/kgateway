import json
import logging
import re
import math
from typing import Union

from .utils import read_mounted_config_map

from pydantic import BaseModel, Field, field_validator

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

logger: logging.Logger = logging.getLogger().getChild(
    "kgateway-ai-ext.external_processor"
)


class WebhookResult:
    REJECTED = "rejected"
    PASSED = "passed"
    MODIFIED = "modified"


class RejectResult:
    REJECTED = "rejected"
    PASSED = "passed"


class Sampler(BaseModel):
    type: str | None = Field(default=None)
    arg: Union[float, str] | None = Field(default=None)

    @field_validator("arg")
    @classmethod
    def validate_arg(cls, v):
        if v is None:
            return v
        if isinstance(v, str):
            try:
                return float(v)
            except ValueError:
                raise ValueError(f"Invalid sampler arg value: {v}")
        return float(v)


class Config(BaseModel):
    """
    Python representation of the AI Tracing config struct from the Gateway Parameters API.

    using pydantic.BaseModel allows for much simpler Json parsing and validation.
    """

    endpoint: str = Field(default="")
    protocol: str | None = Field(default="grpc")
    timeout: Union[int, str] | None = Field(default=None)
    sampler: Sampler | None = Field(default_factory=Sampler)

    @field_validator("timeout")
    @classmethod
    def validate_timeout(cls, v):
        if v is None:
            return v
        if isinstance(v, str):
            if re.fullmatch(r"\d+|(\d+(ms|h|m|s))+", v) is None:
                raise ValueError(f"Invalid timeout format: {v}")

            unit_to_seconds = {
                "": 1,
                "ms": 0.001,
                "s": 1,
                "m": 60,
                "h": 60 * 60,
            }

            # Parse timeout strings like "100s", "5m", "1h", "1s1ms" "1h30m"
            pairs = re.findall(r"(\d+)(ms|h|m|s)?", v)
            duration = sum(int(val) * unit_to_seconds[unit] for val, unit in pairs)

            # timeout should be in milliseconds as per spec, but opentelemetry-python is not aligned with spec.
            # return timeout in seconds here
            #
            # ref: https://github.com/open-telemetry/opentelemetry-python/issues/4044
            return math.ceil(duration)
        return int(v)

    def tracer(self) -> Tracer:
        logger.debug("Initializing OpenTelemetry tracer")

        # Initialize tracer provider
        resource = Resource.create(attributes={SERVICE_NAME: "kgateway-ai-extension"})
        sampler = self._create_sampler()
        tracer_provider = TracerProvider(sampler=sampler, resource=resource)
        # pydantic.BaseModel is creating a default grpc object instead of making it None
        # even the tracing.json file doesn't exist (not configured) and we initialize the
        # Config object with an empty json. So, need the extra checks here to avoid the
        # Tracer error messages and retries when tracing is disabled.
        if len(self.endpoint) > 0:
            logger.info(f"Tracing enabled, publishing to endpoint: {self.endpoint}")
            logger.debug(
                f"Tracing configuration - protocol: {self.protocol}, timeout: {self.timeout}"
            )

            try:
                exporter = self._create_exporter()
                logger.debug(f"Created OTLP exporter for protocol: {self.protocol}")

                # Configure span processor
                span_processor = BatchSpanProcessor(exporter)
                tracer_provider.add_span_processor(span_processor)
                logger.debug("Added BatchSpanProcessor to TracerProvider")

            except Exception as e:
                logger.error(f"Failed to create exporter or span processor: {e}")
        else:
            logger.warning("Using No-op tracer, no endpoint configured")

        return tracer_provider.get_tracer(__name__)

    @classmethod
    def from_file(cls, file_path: str):
        logger.debug(f"Loading tracing configuration from file: {file_path}")
        try:
            data = read_mounted_config_map(file_path)
            return cls(**data)
        except FileNotFoundError as e:
            logger.warning(f"Error: tracing file config was not found: {e}")
            return cls()
        except json.JSONDecodeError as e:
            logger.warning(f"Error: tracing file config is not a valid JSON: {e}")
            return cls()
        except Exception as ex:
            logger.warning(f"An error occurred while reading the file. {ex}")
            return cls()

    def _create_exporter(self):
        """
        Create an OTLP exporter based on the configuration.
        """
        logger.debug(f"Creating OTLP exporter for protocol: {self.protocol}")

        try:
            if self.protocol == "grpc":
                from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import (
                    OTLPSpanExporter,
                )

                logger.debug("Using gRPC OTLP exporter")
                exporter = OTLPSpanExporter(
                    endpoint=self.endpoint,
                    timeout=self.timeout,
                )
                logger.debug(
                    f"Created gRPC exporter - endpoint: {self.endpoint}, timeout: {self.timeout}"
                )
                return exporter
            elif self.protocol in ["http/json", "http/protobuf"]:
                from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
                    OTLPSpanExporter,
                )

                logger.debug("Using HTTP OTLP exporter")
                exporter = OTLPSpanExporter(
                    endpoint=self.endpoint, timeout=self.timeout
                )
                logger.debug(
                    f"Created HTTP exporter - endpoint: {self.endpoint}, timeout: {self.timeout}"
                )
                return exporter
            else:
                error_msg = f"Unsupported protocol: {self.protocol}"
                logger.error(error_msg)
                raise ValueError(error_msg)
        except Exception as e:
            logger.error(f"Failed to create OTLP exporter: {e}")
            raise

    def _create_sampler(self):
        """
        Create a sampler based on the configuration, if unset using ALWAYS_ON sampler.
        """
        sampler_type = self.sampler.type
        sampler_arg = self.sampler.arg

        logger.debug(f"Creating sampler - type: {sampler_type}, arg: {sampler_arg}")

        if sampler_type == "alwaysOn":
            from opentelemetry.sdk.trace.sampling import ALWAYS_ON

            logger.debug("Using ALWAYS_ON sampler")
            return ALWAYS_ON
        elif sampler_type == "alwaysOff":
            from opentelemetry.sdk.trace.sampling import ALWAYS_OFF

            logger.debug("Using ALWAYS_OFF sampler")
            return ALWAYS_OFF
        elif sampler_type == "parentbasedAlwaysOn":
            from opentelemetry.sdk.trace.sampling import DEFAULT_ON

            logger.debug("Using DEFAULT_ON (parent-based always on) sampler")
            return DEFAULT_ON
        elif sampler_type == "parentbasedAlwaysOff":
            from opentelemetry.sdk.trace.sampling import DEFAULT_OFF

            logger.debug("Using DEFAULT_OFF (parent-based always off) sampler")
            return DEFAULT_OFF
        elif sampler_type == "traceidratio":
            from opentelemetry.sdk.trace.sampling import TraceIdRatioBased

            if sampler_arg is None:
                logger.warning(
                    "TraceIdRatioBased sampler requires an 'arg' value. Defaulting to 1.0."
                )
                sampler_arg = 1.0
            logger.debug(f"Using TraceIdRatioBased sampler with ratio: {sampler_arg}")
            return TraceIdRatioBased(sampler_arg)
        elif sampler_type == "parentbasedTraceidratio":
            from opentelemetry.sdk.trace.sampling import ParentBasedTraceIdRatio

            if sampler_arg is None:
                logger.warning(
                    "TraceIdRatioBased sampler requires an 'arg' value. Defaulting to 1.0."
                )
                sampler_arg = 1.0
            logger.debug(
                f"Using ParentBasedTraceIdRatio sampler with ratio: {sampler_arg}"
            )
            return ParentBasedTraceIdRatio(sampler_arg)
        else:
            logger.warning(
                f"Unknown or unspecified sampler type: {sampler_type}. Using default ALWAYS_ON sampler."
            )
            from opentelemetry.sdk.trace.sampling import ALWAYS_ON

            return ALWAYS_ON


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
            logger.info("OtelTracer initialized successfully")
        else:
            logger.warning(
                "OtelTracer is already initialized. Skipping re-initialization."
            )
