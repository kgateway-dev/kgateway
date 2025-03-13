import logging
import pytest
import requests
import json
import os

from openai import Stream
from openai.types.chat.chat_completion_chunk import ChatCompletionChunk

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestStreaming(LLMClient):
    def test_openai_completion_stream(self):
        resp: Stream[ChatCompletionChunk] = self.openai_client.chat.completions.create(
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
            stream=True,
        )
        assert resp is not None
        last_chunk = None
        for chunk in resp:
            logger.debug(f"openai completion stream chunk:\n{chunk}")
            assert chunk is not None
            if len(chunk.choices) > 0:
                last_chunk = chunk
        assert (
            last_chunk is not None
            and len(last_chunk.choices) > 0
            and last_chunk.choices[0].finish_reason == "stop"
        )

    def test_azure_openai_completion_stream(self):
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
            stream=True,
        )
        assert resp is not None
        last_chunk = None
        for chunk in resp:
            logger.debug(f"azure openai completion stream chunk:\n{chunk}")
            assert chunk is not None
            if len(chunk.choices) > 0:
                last_chunk = chunk
        assert (
            last_chunk is not None
            and len(last_chunk.choices) > 0
            and last_chunk.choices[0].finish_reason == "stop"
        )

    # gemini python library does not support parsing alt=sse streaming response. Only supports JSON formatted responses.
    def test_gemini_completion_stream_sse(self):
        payload = {
            "contents": [
                {
                    "role": "user",
                    "parts": [
                        {
                            "text": "Compose a poem that explains the concept of recursion in programming.",
                        }
                    ],
                }
            ]
        }
        gemini_url = os.environ.get("TEST_GEMINI_BASE_URL", "")

        # Send a request to the URL with streaming enabled
        with requests.post(gemini_url, json=payload, stream=True) as response:
            assert response.status_code == 200, "Failed to get a successful response"
            assert response.headers.get("Content-Type") == "text/event-stream", (
                "Unexpected content type"
            )

        # Process chunks
        for chunk in response.iter_lines(decode_unicode=True):
            if chunk:  # Skip empty lines
                try:
                    data = json.loads(chunk)
                    assert "candidates" in data, "Missing 'candidates' key in the data"
                    assert len(data["candidates"]) > 0, (
                        "'candidates' should not be empty"
                    )

                    # Verify that each chunk has reasonable content
                    for candidate in data["candidates"]:
                        assert "content" in candidate, "Missing 'content' in candidate"
                        content = candidate["content"]
                        if (
                            candidate.get("finishReason", None) is not None
                            and "parts" not in content
                        ):
                            continue
                        assert "parts" in content, "Missing 'parts' in content"
                        assert len(content["parts"]) > 0, "'parts' should not be empty"
                        for part in content["parts"]:
                            assert isinstance(part["text"], str), (
                                "Expected 'text' to be a string"
                            )
                            assert len(part["text"]) > 0, "'text' should not be empty"
                except ValueError as e:
                    pytest.fail(f"Invalid JSON received: {e}")

    # vertex ai python library does not support parsing alt=sse streaming response. Only supports JSON formatted responses.
    def test_vertex_ai_completion_stream_sse(self):
        payload = {
            "contents": [
                {
                    "role": "user",
                    "parts": [
                        {
                            "text": "Compose a poem that explains the concept of recursion in programming.",
                        }
                    ],
                }
            ]
        }
        vertexai_url = os.environ.get("TEST_VERTEX_AI_BASE_URL", "")

        # Send a request to the URL with streaming enabled
        with requests.post(vertexai_url, json=payload, stream=True) as response:
            assert response.status_code == 200, "Failed to get a successful response"
            assert response.headers.get("Content-Type") == "text/event-stream", (
                "Unexpected content type"
            )

        # Process chunks
        for chunk in response.iter_lines(decode_unicode=True):
            if chunk:  # Skip empty lines
                try:
                    data = json.loads(chunk)
                    assert "candidates" in data, "Missing 'candidates' key in the data"
                    assert len(data["candidates"]) > 0, (
                        "'candidates' should not be empty"
                    )

                    # Verify that each chunk has reasonable content
                    for candidate in data["candidates"]:
                        assert "content" in candidate, "Missing 'content' in candidate"
                        content = candidate["content"]
                        if (
                            candidate.get("finishReason", None) is not None
                            and "parts" not in content
                        ):
                            continue
                        assert "parts" in content, "Missing 'parts' in content"
                        assert len(content["parts"]) > 0, "'parts' should not be empty"
                        for part in content["parts"]:
                            assert isinstance(part["text"], str), (
                                "Expected 'text' to be a string"
                            )
                            assert len(part["text"]) > 0, "'text' should not be empty"
                except ValueError as e:
                    pytest.fail(f"Invalid JSON received: {e}")
