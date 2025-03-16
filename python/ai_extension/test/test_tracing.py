import pytest
from opentelemetry.trace import Tracer, NoOpTracer
from telemetry.tracing import Config as TraceConfig, OtelTracer


@pytest.fixture
def tracing_config():
    return TraceConfig.from_file("test/test_data/tracing_config.b64")


class TestTracing:
    def test_from_file(self, tracing_config):
        assert tracing_config.grpc is not None
        assert tracing_config.grpc.host == "1.2.3.4"
        assert tracing_config.grpc.port == 1234
        assert tracing_config.insecure

    def test_tracer_global_access(self, tracing_config):
        assert isinstance(OtelTracer.get(), NoOpTracer)

        OtelTracer.init(tracing_config.tracer())

        assert isinstance(OtelTracer.get(), Tracer)
