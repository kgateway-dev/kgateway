import asyncio
import json
import logging
import pytest

from telemetry.stats import Config as StatsConfig
from telemetry.tracing import Config as TracingConfig
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
