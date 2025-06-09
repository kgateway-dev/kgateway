import logging
import os
import pytest
import requests
import google.api_core.retry as google_retry
from google.api_core.exceptions import (
    GoogleAPIError,
    DeadlineExceeded,
)
from tenacity import (
    retry,
    stop_after_attempt,
    wait_exponential,
    retry_if_exception_type,
)
from google.generativeai.types import helper_types
from google.generativeai.types.answer_types import FinishReason as GeminiFinishReason
from vertexai.generative_models import FinishReason as VertexFinishReason

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestRouting(LLMClient):
    def test_openai_completion(self):
        resp = self.openai_chat_completion(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "system",
                    "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair.",
                },
                {
                    "role": "user",
                    "content": "Compose a poem that explains the concept of recursion in programming.",
                },
            ],
        )
        logger.debug(f"openai routing response:\n{resp}")
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
        )
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_azure_openai_completion(self):
        resp = self.azure_openai_client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "system",
                    "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair.",
                },
                {
                    "role": "user",
                    "content": "Compose a poem that explains the concept of recursion in programming.",
                },
            ],
        )
        logger.debug(f"azure routing response:\n{resp}")
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
        )
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    @pytest.mark.skipif(
        os.environ.get("TEST_TOKEN_PASSTHROUGH") == "true",
        reason="passthrough not enabled for gemini",
    )
    @pytest.mark.skipif(
        os.environ.get("TEST_OVERRIDE_PROVIDER") == "true",
        reason="overrideProvider not enabled for gemini",
    )
    def test_gemini_completion(self):
        resp = self.gemini_client.generate_content(
            "Compose a poem that explains the concept of recursion in programming.",
            request_options=helper_types.RequestOptions(
                retry=google_retry.Retry(
                    initial=10, multiplier=2, maximum=60, timeout=300
                )
            ),
        )
        assert resp is not None
        logger.debug(f"gemini routing response:\n{resp}")
        assert len(resp.candidates) == 1
        assert resp.candidates[0].finish_reason == GeminiFinishReason.STOP
        assert resp.usage_metadata.prompt_token_count > 0

    # Retry on transient errors with exponential backoff
    @retry(
        retry=retry_if_exception_type(
            (GoogleAPIError, DeadlineExceeded, requests.exceptions.ConnectionError)
        ),
        stop=stop_after_attempt(10),
        wait=wait_exponential(multiplier=1, min=2, max=10),
    )
    def test_vertex_ai_completion(self):
        resp = self.vertex_ai_client.generate_content(
            "Compose a poem that explains the concept of recursion in programming."
        )
        assert resp is not None
        logger.debug(f"Vertex AI routing response:\n{resp}")
        assert len(resp.candidates) == 1
        assert resp.candidates[0].finish_reason == VertexFinishReason.STOP
        assert resp.usage_metadata.prompt_token_count > 0
