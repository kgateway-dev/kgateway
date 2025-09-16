import asyncio
import json
import logging
from typing import Optional

import pytest

from guardrails.presidio import init_presidio_config
from python.ai_extension.api.kgateway.policy.ai import prompt_guard
import telemetry.attributes as ai_attributes
from telemetry.stats import Config as StatsConfig
from telemetry.tracing import Config as TracingConfig, OtelTracer
from ext_proc.server import StreamHandler, ExtProcServer
from api.envoy.service.ext_proc.v3 import external_processor_pb2
from prometheus_client import CollectorRegistry, Counter
from openai.types.moderation_create_response import ModerationCreateResponse
from openai.types.moderation import (
    Moderation,
    Categories,
    CategoryScores,
    CategoryAppliedInputTypes,
)

from openai.resources import AsyncModerations
from openai import AsyncOpenAI
from opentelemetry import trace
from opentelemetry.semconv._incubating.attributes import gen_ai_attributes
from opentelemetry.sdk.trace import TracerProvider, Span
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.trace import set_tracer_provider


class MockAsyncOpenAI(AsyncOpenAI):
    def __init__(self, client):
        pass


class MockModerationClient(AsyncModerations):
    async def create(self, **params):
        response = ModerationCreateResponse(
            id="test-id",
            model="test-model",
            results=[
                Moderation(
                    flagged=True,
                    categories=Categories.construct(),
                    category_scores=CategoryScores.construct(),
                    category_applied_input_types=CategoryAppliedInputTypes.construct(),
                )
            ],
        )
        return response


logger = logging.getLogger().getChild("server-test-logger")

# Create a new CollectorRegistry to ensure no previous collectors are registered
test_registry = CollectorRegistry()

# Example of creating a Counter with the new registry
ai_prompt_tokens = Counter("ai_prompt_tokens", "Description", registry=test_registry)

stats_config = StatsConfig(customLabels=[])
tracing_config = TracingConfig()
extproc_server = ExtProcServer(stats_config, tracing_config)


@pytest.mark.parametrize(
    "req_body_content, llm_provider",
    [
        (
            b"""{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "Are you ok?"
    }
  ],
  "stream": false
}""",
            "openai",
        ),
        (
            b"""{
  "contents":[
     {
        "role":"user",
        "parts":[
           {
              "text":"Gemini is cool!"
           }
        ]
     }
  ]
}""",
            "gemini",
        ),
    ],
)
def test_handle_request_body(req_body_content, llm_provider):
    req_body = external_processor_pb2.HttpBody(
        body=req_body_content,
        end_of_stream=True,
    )
    metadict = {"x-llm-provider": llm_provider}

    handler = StreamHandler.from_metadata(metadict)
    handler.req_webhook = None
    handler.req.set_headers(
        headers=external_processor_pb2.HttpHeaders(), header_rules=[]
    )

    response = asyncio.run(
        extproc_server.handle_request_body(
            req_body,
            metadict,
            handler,
            trace.NonRecordingSpan(trace.SpanContext(0, 0, False)),
        )
    )

    assert response is not None
    assert isinstance(response, external_processor_pb2.ProcessingResponse)
    assert json.loads(
        response.request_body.response.body_mutation.body.decode("utf-8")
    ) == json.loads(req_body.body.decode("utf-8"))
    assert "envoy.ratelimit" in response.dynamic_metadata.fields
    ratelimit = response.dynamic_metadata.fields["envoy.ratelimit"].struct_value
    assert "hits_addend" in ratelimit.fields


def test_handle_request_headers():
    headers = external_processor_pb2.HttpHeaders()
    headers.headers.headers.add(key="authorization", value="test-auth-token")

    metadict = {"x-llm-provider": "openai"}
    handler = StreamHandler.from_metadata(metadict)
    handler.req.headers = {
        header.key: header.value for header in headers.headers.headers
    }

    response = extproc_server.handle_request_headers(headers, handler)
    assert "ai.kgateway.io" in response.dynamic_metadata.fields
    ai_metadata = response.dynamic_metadata.fields["ai.kgateway.io"].struct_value
    assert "auth_token" in ai_metadata.fields


@pytest.mark.parametrize(
    "req_body_content, llm_provider",
    [
        (
            b"""{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "Are you ok?"
    }
  ],
  "stream": false
}""",
            "openai",
        ),
        (
            b"""{
  "contents":[
     {
        "role":"user",
        "parts":[
           {
              "text":"Gemini is cool!"
           }
        ]
     }
  ]
}""",
            "gemini",
        ),
    ],
)
def test_handle_request_body_with_moderation(req_body_content, llm_provider):
    req_body = external_processor_pb2.HttpBody(
        body=req_body_content,
        end_of_stream=True,
    )
    metadict = {"x-llm-provider": llm_provider}

    handler = StreamHandler.from_metadata(metadict)
    handler.req_webhook = None
    handler.req.set_headers(
        headers=external_processor_pb2.HttpHeaders(), header_rules=[]
    )
    handler.req_moderation = (
        MockModerationClient(client=MockAsyncOpenAI(client=None)),
        "test-model",
    )
    response = asyncio.run(
        extproc_server.handle_request_body(
            req_body,
            metadict,
            handler,
            trace.NonRecordingSpan(trace.SpanContext(0, 0, False)),
        )
    )

    assert response.immediate_response.body == b"Rejected by guardrails moderation"


@pytest.mark.parametrize(
    "req_body_content, resp_body_content, llm_provider",
    [
        (
            b"""{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "Are you ok?"
    }
  ],
  "stream": false
}""",
            """{
   "id":"fake",
   "object":"chat.completion",
   "created":1722966273,
   "model":"gpt-4o-mini-2024-07-18",
   "choices":[
      {
         "index":0,
         "message":{
            "role":"assistant",
            "content":"Say hello to the world!",
            "refusal":null
         },
         "logprobs":null,
         "finish_reason":"stop"
      }
   ],
   "usage":{
      "prompt_tokens":11,
      "completion_tokens":310,
      "total_tokens":321
   },
   "system_fingerprint":"fp_48196bc67a"
}""",
            "openai",
        ),
        (
            b"""{
  "contents":[
     {
        "role":"user",
        "parts":[
           {
              "text":"Gemini is cool!"
           }
        ]
     }
  ]
}""",
            """{
  "candidates":[
     {
        "content":{
           "role":"model",
           "parts":[
              {
                 "text":"Say hello to the world!"
              }
           ]
        }
     }
  ],
  "modelVersion":"gemini-1.5-flash-001",
  "usageMetadata": {
    "promptTokenCount": 4,
    "candidatesTokenCount": 491,
    "totalTokenCount": 495
  }
}""",
            "gemini",
        ),
    ],
)
def test_handle_response_body_with_end_of_stream(
    req_body_content, resp_body_content, llm_provider
):
    headers = external_processor_pb2.HttpHeaders()
    headers.headers.headers.add(key="authorization", value="test-auth-token")

    metadict = {"x-llm-provider": llm_provider}
    handler = StreamHandler.from_metadata(metadict)
    handler.req.headers = {
        header.key: header.value for header in headers.headers.headers
    }

    handler.req.append(req_body_content)

    resp_body = external_processor_pb2.HttpBody(
        body=resp_body_content.encode("utf-8"), end_of_stream=True
    )
    response = asyncio.run(
        extproc_server.handle_response_body(
            resp_body, handler, trace.NonRecordingSpan(trace.SpanContext(0, 0, False))
        )
    )

    assert response is not None
    assert isinstance(response, external_processor_pb2.ProcessingResponse)
    assert json.loads(
        response.response_body.response.body_mutation.body.decode("utf-8")
    ) == json.loads(resp_body_content)


@pytest.fixture
def setup_in_memory_tracer():
    """Setup in-memory span exporter for testing instrumentation"""

    # Create in-memory exporter
    memory_exporter = InMemorySpanExporter()

    # Create tracer provider with in-memory exporter
    tracer_provider = TracerProvider()
    tracer_provider.add_span_processor(SimpleSpanProcessor(memory_exporter))

    # Set as global tracer provider
    set_tracer_provider(tracer_provider)

    # Force override the existing tracer (bypassing init protection logic)
    test_tracer = tracer_provider.get_tracer("test-tracer")
    OtelTracer._OtelTracer__tracer = test_tracer

    # Verify override success
    current_tracer = OtelTracer.get()
    assert current_tracer is test_tracer, "Failed to override tracer"

    yield memory_exporter, test_tracer

    memory_exporter.clear()


def stream_handler(metadict: dict) -> StreamHandler:
    """
    Create a common StreamHandler instance for testing.

    Args:
        metadict: Metadata dictionary containing LLM provider info

    Returns:
        Configured StreamHandler instance
    """
    headers = external_processor_pb2.HttpHeaders()

    handler = StreamHandler.from_metadata(metadict)
    handler.req.set_headers(headers=headers, header_rules=[])
    handler.resp.set_headers(headers=headers, header_rules=[])
    return handler


class TestInstrumentation:
    # TODO(zhengke): Refactor the test fixture to simplify testing for multiple providers,just like provider_response_test_data
    @pytest.fixture(scope="class")
    def request_body_content(self):
        """Fixture for common OpenAI request body content."""
        return {
            "model": "openai/gpt-4o",
            "messages": [
                {
                    "role": "system",
                    "content": "You are a helpful assistant that answers questions concisely.",
                },
                {
                    "role": "user",
                    "content": "What is the meaning of life? Please elaborate in a few sentences.",
                },
            ],
            "response_format": {"type": "text"},
            "n": 2,
            "seed": 12345,
            "frequency_penalty": 0.5,
            "max_tokens": 150,
            "presence_penalty": 0.3,
            "stop": ["\n\n", "END"],
            "temperature": 0.7,
            "top_k": 50,
            "top_p": 0.9,
        }

    @pytest.fixture(scope="class")
    def provider_response_test_data(self):
        """
        Unified provider test data for table-driven testing.

        This fixture contains response body examples and expected OTel attributes
        for each supported AI provider (OpenAI, Gemini, Anthropic). Each provider entry
        includes:
        - response_body: A realistic JSON response from the provider's API
        - expected_attributes: The corresponding OTel attributes that should
          be extracted from the response body

        This approach enables parameterized testing across multiple providers while
        maintaining consistency in test structure and validation logic.
        """
        return {
            "openai": {
                "response_body": """{
                        "id": "fake",
                        "object": "chat.completion",
                        "created": 1722966273,
                        "model": "gpt-4o-mini-2024-07-18",
                        "choices": [
                            {
                                "index": 0,
                                "message": {
                                "role": "assistant",
                                "content": "Say hello to the world!",
                                "refusal": null
                            },
                            "logprobs": null,
                            "finish_reason": "stop"
                            }
                        ],
                        "usage": {
                            "prompt_tokens": 11,
                            "completion_tokens": 310,
                            "total_tokens": 321
                        },
                        "system_fingerprint": "fp_48196bc67a"
                    }""",
                "expected_attributes": {
                    gen_ai_attributes.GEN_AI_OPERATION_NAME: "chat",
                    gen_ai_attributes.GEN_AI_SYSTEM: "openai",
                    gen_ai_attributes.GEN_AI_RESPONSE_ID: "fake",
                    gen_ai_attributes.GEN_AI_RESPONSE_MODEL: "gpt-4o-mini-2024-07-18",
                    gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS: "stop",
                    gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS: 11,
                    gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS: 310,
                },
            },
            "gemini": {
                "response_body": """{
                        "candidates": [
                            {
                                "content": {
                                    "parts": [
                                        {
                                            "text": "hello, world"
                                        }
                                    ],
                                    "role": "model"
                                },
                                "finishReason": "STOP",
                                "index": 0
                            }
                        ],
                        "usageMetadata": {
                            "promptTokenCount": 5,
                            "candidatesTokenCount": 1298,
                            "totalTokenCount": 2356,
                            "promptTokensDetails": [
                                {
                                    "modality": "TEXT",
                                    "tokenCount": 5
                                }
                            ],
                            "thoughtsTokenCount": 1053
                        },
                        "modelVersion": "gemini-2.5-flash",
                        "responseId": "tYmZaMTQLcayqtsP_rq7gQs"
                    }""",
                "expected_attributes": {
                    gen_ai_attributes.GEN_AI_OPERATION_NAME: "generate_content",
                    gen_ai_attributes.GEN_AI_SYSTEM: "gcp.gemini",
                    gen_ai_attributes.GEN_AI_RESPONSE_ID: "tYmZaMTQLcayqtsP_rq7gQs",
                    gen_ai_attributes.GEN_AI_RESPONSE_MODEL: "gemini-2.5-flash",
                    gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS: "STOP",
                    gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS: 5,
                    gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS: 1298,
                },
            },
            "anthropic": {
                "response_body": """{
                        "id": "msg_01XFDUDYJgAACzvnptvVoYEL",
                        "type": "message",
                        "role": "assistant",
                        "content": [
                            {
                              "type": "text",
                              "text": "Hello!"
                            }
                        ],
                        "model": "claude-opus-4-1-20250805",
                        "stop_reason": "end_turn",
                        "stop_sequence": null,
                        "usage": {
                            "input_tokens": 12,
                            "output_tokens": 6
                        }
                    }""",
                "expected_attributes": {
                    gen_ai_attributes.GEN_AI_OPERATION_NAME: "generate_content",
                    gen_ai_attributes.GEN_AI_SYSTEM: "anthropic",
                    gen_ai_attributes.GEN_AI_RESPONSE_ID: "msg_01XFDUDYJgAACzvnptvVoYEL",
                    gen_ai_attributes.GEN_AI_RESPONSE_MODEL: "claude-opus-4-1-20250805",
                    gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS: "end_turn",
                    gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS: 12,
                    gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS: 6,
                },
            },
        }

    @pytest.fixture
    def base_handler(self):
        metadict = {"x-llm-provider": "openai"}
        handler = stream_handler(metadict)
        handler.req.path = "/chat/completions"
        return handler

    # ========== Helper Methods ==========
    def _create_request_body(
        self, content: dict[str, any]
    ) -> external_processor_pb2.HttpBody:
        return external_processor_pb2.HttpBody(
            body=json.dumps(content).encode("utf-8"),
            end_of_stream=True,
        )

    def _create_response_body(self, content: str) -> external_processor_pb2.HttpBody:
        return external_processor_pb2.HttpBody(
            body=content.encode("utf-8"), end_of_stream=True
        )

    def _execute_request_body_test(
        self,
        test_tracer: trace.Tracer,
        req_body: external_processor_pb2.HttpBody,
        handler: StreamHandler,
        metadict: Optional[dict[str, str]] = None,
    ) -> external_processor_pb2.ProcessingResponse:
        if metadict is None:
            metadict = {"x-llm-provider": "openai"}

        with test_tracer.start_as_current_span("test_parent_span"):
            response = asyncio.run(
                extproc_server.handle_request_body(
                    req_body,
                    metadict,
                    handler,
                    None,
                )
            )
        return response

    def _execute_response_body_test(
        self,
        test_tracer: trace.Tracer,
        resp_body: external_processor_pb2.HttpBody,
        handler: StreamHandler,
    ) -> external_processor_pb2.ProcessingResponse:
        with test_tracer.start_as_current_span("test_parent_span"):
            response = asyncio.run(
                extproc_server.handle_response_body(
                    resp_body,
                    handler,
                    None,
                )
            )
        return response

    def _verify_basic_response(
        self, response: external_processor_pb2.ProcessingResponse
    ):
        """verify basic response"""
        assert response is not None, "Response should not be None"
        assert isinstance(response, external_processor_pb2.ProcessingResponse), (
            "Response should be ProcessingResponse instance"
        )

    def _find_span_by_name_prefix(self, spans: list, name_prefix: str) -> Span:
        """Find span by name prefix"""
        span = next((s for s in spans if s.name.startswith(name_prefix)), None)
        assert span is not None, f"Expected a {name_prefix} span to be created"
        return span

    def _verify_tracer_setup(self, test_tracer, setup_in_memory_tracer):
        """Verify tracer setup"""
        memory_exporter, tracer = setup_in_memory_tracer
        current_tracer = OtelTracer.get()
        assert current_tracer is test_tracer, (
            "Current tracer does not match test tracer"
        )
        return memory_exporter

    def _create_webhook_config(self, host: str = "example.com", port: int = 443):
        """Create webhook config"""
        return prompt_guard.Webhook.from_json(
            {
                "host": {"host": host, "port": port},
                "forwardHeaders": [
                    {
                        "type": "Exact",
                        "name": "Authorization",
                        "value": "Bearer test-token",
                    }
                ],
            }
        )

    def _create_parameterized_moderation_client(self, flagged: bool, model: str):
        """Create parameterized moderation client"""

        class ParameterizedMockModerationClient(AsyncModerations):
            async def create(self, **params):
                return ModerationCreateResponse(
                    id="test-id",
                    model=model,
                    results=[
                        Moderation(
                            flagged=flagged,
                            categories=Categories.construct(),
                            category_scores=CategoryScores.construct(),
                            category_applied_input_types=CategoryAppliedInputTypes.construct(),
                        )
                    ],
                )

        return ParameterizedMockModerationClient(client=MockAsyncOpenAI(client=None))

    # ========== Test Methods ==========
    def test_handle_request_body(
        self, setup_in_memory_tracer, request_body_content, base_handler
    ):
        """Test that request body handling creates proper spans with attributes"""
        memory_exporter, test_tracer = setup_in_memory_tracer
        self._verify_tracer_setup(test_tracer, setup_in_memory_tracer)

        req_body = self._create_request_body(request_body_content)
        base_handler.req_webhook = None

        response = self._execute_request_body_test(test_tracer, req_body, base_handler)
        self._verify_basic_response(response)

        # Verify instrumentation
        spans = memory_exporter.get_finished_spans()
        assert len(spans) >= 1, "Expected at least one span to be created"

        gen_ai_client_span = self._find_span_by_name_prefix(spans, "gen_ai.request")

        # Verify attributes
        attributes = gen_ai_client_span.attributes
        self._verify_request_attributes(attributes, request_body_content, base_handler)

    def _verify_request_attributes(self, attributes, request_body_content, handler):
        """Verify request attributes"""
        assert attributes.get(gen_ai_attributes.GEN_AI_OPERATION_NAME) == "chat"
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_SYSTEM) == handler.get_ai_system()
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_OUTPUT_TYPE)
            == request_body_content["response_format"]["type"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_CHOICE_COUNT)
            == request_body_content["n"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_MODEL)
            == handler.request_model
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_SEED)
            == request_body_content["seed"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_FREQUENCY_PENALTY)
            == request_body_content["frequency_penalty"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_MAX_TOKENS)
            == request_body_content["max_tokens"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_PRESENCE_PENALTY)
            == request_body_content["presence_penalty"]
        )
        assert attributes.get(gen_ai_attributes.GEN_AI_REQUEST_STOP_SEQUENCES) == tuple(
            request_body_content["stop"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_TEMPERATURE)
            == request_body_content["temperature"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_TOP_K)
            == request_body_content["top_k"]
        )
        assert (
            attributes.get(gen_ai_attributes.GEN_AI_REQUEST_TOP_P)
            == request_body_content["top_p"]
        )

    @pytest.mark.parametrize(
        "webhook_response,expected_result",
        [
            ({"action": {"type": "pass"}}, "passed"),
            (
                {
                    "action": {
                        "type": "modify",
                        "body": {"messages": [{"role": "user", "content": "modified"}]},
                    }
                },
                "modified",
            ),
            (
                {
                    "action": {
                        "type": "reject",
                        "reason": "test",
                        "status_code": 400,
                        "body": "rejected",
                    }
                },
                "rejected",
            ),
        ],
        ids=["pass", "modify", "reject"],
    )
    def test_handle_request_body_webhook(
        self,
        httpx_mock,
        setup_in_memory_tracer,
        request_body_content,
        base_handler,
        webhook_response,
        expected_result,
    ):
        """Test webhook instrumentation with mocked HTTP client for different scenarios."""
        memory_exporter, test_tracer = setup_in_memory_tracer
        self._verify_tracer_setup(test_tracer, setup_in_memory_tracer)

        # Setup HTTP mock
        httpx_mock.add_response(
            url="https://example.com:443/request",
            method="POST",
            json=webhook_response,
            status_code=200,
        )

        req_body = self._create_request_body(request_body_content)
        base_handler.req_webhook = self._create_webhook_config()

        response = self._execute_request_body_test(test_tracer, req_body, base_handler)
        self._verify_basic_response(response)

        spans = memory_exporter.get_finished_spans()
        webhook_span = self._find_span_by_name_prefix(
            spans, "handle_request_body_req_webhook"
        )

        self._verify_webhook_attributes(webhook_span, base_handler, expected_result)

    def _verify_webhook_attributes(self, span, handler, expected_result):
        """Verify webhook attributes"""
        attributes = span.attributes
        assert attributes is not None, "Webhook span attributes should not be None"
        assert attributes.get(ai_attributes.AI_WEBHOOK_ENDPOINT) == str(
            handler.req_webhook.endpoint
        )
        assert attributes.get(ai_attributes.AI_WEBHOOK_RESULT) == expected_result

    @pytest.mark.parametrize(
        "regex_config,test_content,expected_result",
        [
            (
                {
                    "matches": [{"pattern": r"\d{3}-\d{3}-\d{4}", "name": "phone"}],
                    "builtins": ["PHONE_NUMBER"],
                    "action": "MASK",
                },
                "My phone number is 123-456-7890",
                "passed",
            ),
            (
                {
                    "matches": [
                        {"pattern": r"\d{4}-\d{4}-\d{4}-\d{4}", "name": "creditcard"}
                    ],
                    "builtins": ["CREDIT_CARD"],
                    "action": "REJECT",
                },
                "My credit card number is 4532-1234-5678-9012",
                "passed",
            ),
        ],
        ids=["phone_mask", "credit_card_reject"],
    )
    def test_handle_request_body_regex(
        self,
        setup_in_memory_tracer,
        request_body_content,
        base_handler,
        regex_config,
        test_content,
        expected_result,
    ):
        """Test regex filtering instrumentation with different patterns and actions."""
        memory_exporter, test_tracer = setup_in_memory_tracer
        self._verify_tracer_setup(test_tracer, setup_in_memory_tracer)

        req_body = self._create_request_body(request_body_content)

        # Configure regex filtering
        regex_config_obj = prompt_guard.Regex.from_json(regex_config)
        base_handler.req_regex = init_presidio_config(regex_config_obj)
        base_handler.req_regex_action = regex_config_obj.action

        response = self._execute_request_body_test(test_tracer, req_body, base_handler)
        self._verify_basic_response(response)

        spans = memory_exporter.get_finished_spans()
        regex_span = self._find_span_by_name_prefix(
            spans, "handle_request_body_req_regex"
        )

        self._verify_regex_attributes(regex_span, regex_config_obj, expected_result)

    def _verify_regex_attributes(self, span, regex_config, expected_result):
        """Verify regex attributes"""
        assert (
            span.attributes.get(ai_attributes.AI_REGEX_ACTION)
            == regex_config.action.value
        )
        assert span.attributes.get(ai_attributes.AI_REGEX_RESULT) == expected_result

    @pytest.mark.parametrize(
        "moderation_flagged,expected_result",
        [
            (False, "passed"),
            (True, "rejected"),
        ],
        ids=["moderation_pass", "moderation_reject"],
    )
    def test_handle_request_body_moderation(
        self,
        setup_in_memory_tracer,
        request_body_content,
        base_handler,
        moderation_flagged,
        expected_result,
    ):
        """Test content moderation instrumentation with different flagging scenarios."""
        memory_exporter, test_tracer = setup_in_memory_tracer
        self._verify_tracer_setup(test_tracer, setup_in_memory_tracer)

        model = "text-moderation-latest"
        req_body = self._create_request_body(request_body_content)

        base_handler.req_webhook = None
        base_handler.req_regex = None
        base_handler.req_moderation = (
            self._create_parameterized_moderation_client(moderation_flagged, model),
            model,
        )

        response = self._execute_request_body_test(test_tracer, req_body, base_handler)
        self._verify_basic_response(response)

        self._verify_moderation_response_behavior(response, expected_result)

        spans = memory_exporter.get_finished_spans()
        moderation_span = self._find_span_by_name_prefix(
            spans, "handle_request_body_req_moderation"
        )

        self._verify_moderation_attributes(moderation_span, model, moderation_flagged)

    def _verify_moderation_response_behavior(self, response, expected_result):
        """Verify moderation response behavior"""
        if expected_result == "rejected":
            assert hasattr(response, "immediate_response")
            assert response.immediate_response is not None
            assert (
                response.immediate_response.body == b"Rejected by guardrails moderation"
            )
        else:
            assert hasattr(response, "request_body")
            assert response.request_body is not None

    def _verify_moderation_attributes(self, span, model, flagged):
        """Verify moderation attributes"""
        attributes = span.attributes
        assert attributes.get(ai_attributes.AI_MODERATION_MODEL) == model
        assert attributes.get(ai_attributes.AI_MODERATION_FLAGGED) == flagged

    @pytest.mark.parametrize(
        "webhook_response,expected_result",
        [
            ({"action": {"type": "pass"}}, "passed"),
            (
                {
                    "action": {
                        "body": {
                            "choices": [
                                {
                                    "message": {
                                        "role": "Assistant",
                                        "content": "1 + 2 is 3",
                                    }
                                }
                            ]
                        }
                    }
                },
                "modified",
            ),
        ],
        ids=["response_webhook_pass", "response_webhook_modify"],
    )
    def test_handle_response_body_webhook(
        self,
        setup_in_memory_tracer,
        httpx_mock,
        provider_response_test_data,
        base_handler,
        webhook_response,
        expected_result,
    ):
        """Test response body webhook processing with instrumentation."""
        memory_exporter, test_tracer = setup_in_memory_tracer
        self._verify_tracer_setup(test_tracer, setup_in_memory_tracer)

        # Mock HTTP response
        httpx_mock.add_response(
            url="https://example.com:443/response",
            method="POST",
            json=webhook_response,
            status_code=200,
        )

        test_data = provider_response_test_data["openai"]
        resp_body = self._create_response_body(test_data["response_body"])
        base_handler.resp_webhook = self._create_webhook_config()

        response = self._execute_response_body_test(
            test_tracer, resp_body, base_handler
        )
        self._verify_basic_response(response)

        spans = memory_exporter.get_finished_spans()
        webhook_span = self._find_span_by_name_prefix(
            spans, "handle_response_body_resp_webhook"
        )

        self._verify_response_webhook_attributes(
            webhook_span, base_handler, expected_result
        )

    def _verify_response_webhook_attributes(self, span, handler, expected_result):
        """Verify response webhook attributes"""
        attributes = span.attributes
        assert attributes.get(ai_attributes.AI_WEBHOOK_ENDPOINT) == str(
            handler.resp_webhook.endpoint
        )
        assert attributes.get(ai_attributes.AI_WEBHOOK_RESULT) == expected_result

    @pytest.mark.parametrize(
        "provider,path",
        [
            ("openai", "/chat/completions"),
            ("gemini", "/v1beta/models/gemini-2.5-flash:generateContent"),
            ("anthropic", "api.anthropic.com/v1/messages"),
        ],
        ids=["openai", "gemini", "anthropic"],
    )
    def test_handle_response(
        self, setup_in_memory_tracer, provider_response_test_data, provider, path
    ):
        """Test response body handling for all providers"""
        memory_exporter, test_tracer = setup_in_memory_tracer

        metadict = {"x-llm-provider": provider}
        handler = stream_handler(metadict)
        handler.req.path = path
        handler.resp_webhook = None

        test_data = provider_response_test_data[provider]
        resp_body = self._create_response_body(test_data["response_body"])

        response = self._execute_response_body_test(test_tracer, resp_body, handler)
        self._verify_basic_response(response)

        spans = memory_exporter.get_finished_spans()
        gen_ai_response_span = self._find_span_by_name_prefix(spans, "gen_ai.response")

        self._verify_provider_response_attributes(
            gen_ai_response_span.attributes, test_data["expected_attributes"]
        )

    def _verify_provider_response_attributes(self, actual_attrs, expected_attrs):
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_OPERATION_NAME
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_OPERATION_NAME)
        assert actual_attrs.get(gen_ai_attributes.GEN_AI_SYSTEM) == expected_attrs.get(
            gen_ai_attributes.GEN_AI_SYSTEM
        )
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_RESPONSE_ID
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_RESPONSE_ID)
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_RESPONSE_MODEL
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_RESPONSE_MODEL)
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS)
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS)
        assert actual_attrs.get(
            gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS
        ) == expected_attrs.get(gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS)


def test_webhook_config_parsing():
    req_webhook_json = """
    {
        "webhook": {
            "host": {
                "host": "ai-guardrail-webhook.kgateway-system.svc.cluster.local",
                "port": 8000
            }
        }
    }
    """
    
    metadict = {
        "x-llm-provider": "openai",
        "x-req-guardrails-config": req_webhook_json,
        "x-req-guardrails-config-hash": "test-hash"
    }
    
    headers = external_processor_pb2.HttpHeaders()
    handler = asyncio.run(
        extproc_server.parse_handler_config(
            StreamHandler.from_metadata(metadict), metadict, headers
        )
    )
    
    assert handler.req_webhook is not None
    assert handler.req_webhook.endpoint.host == "ai-guardrail-webhook.kgateway-system.svc.cluster.local"
    assert handler.req_webhook.endpoint.port == 8000
    
    resp_webhook_json = """
    {
        "webhook": {
            "host": {
                "host": "response-webhook.example.com",
                "port": 9000
            }
        }
    }
    """
    
    metadict_resp = {
        "x-llm-provider": "openai",
        "x-resp-guardrails-config": resp_webhook_json,
        "x-resp-guardrails-config-hash": "test-hash-resp"
    }
    
    handler_resp = asyncio.run(
        extproc_server.parse_handler_config(
            StreamHandler.from_metadata(metadict_resp), metadict_resp, headers
        )
    )
    
    assert handler_resp.resp_webhook is not None
    assert handler_resp.resp_webhook.endpoint.host == "response-webhook.example.com"
    assert handler_resp.resp_webhook.endpoint.port == 9000
