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

from google.generativeai.types import generation_types
from google.generativeai.types import helper_types
from google.generativeai.types.answer_types import FinishReason as GeminiFinishReason
from vertexai.generative_models import FinishReason as VertexFinishReason

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestRouting(LLMClient):
    def test_openai_completion(self):
        resp = self.openai_client.chat.completions.create(
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
            response_format={"type": "text"},
            n=2,
            seed=12345,
            frequency_penalty=0.5,
            max_tokens=150,
            presence_penalty=0.3,
            stop=["\n\n", "END"],
            temperature=0.7,
            top_p=0.9,
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
            contents="Write a short story about a detective and a mysterious case.",
            generation_config=generation_types.GenerationConfig(
                stop_sequences=["THE END", "end of story."],
                candidate_count=1,
                max_output_tokens=5000,
                temperature=0.9,
                top_p=0.95,
                top_k=40,
                frequency_penalty=0.5,
                presence_penalty=0.3,
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
