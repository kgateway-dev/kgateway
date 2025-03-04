import logging
import pytest
import requests
import os
from openai import PermissionDeniedError

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

test_paragraph = "Envoy is a self contained, high performance server with a small memory footprint. It runs alongside any application language or framework. HTTP/2 and gRPC support. Envoy has first class support for HTTP/2 and gRPC for both incoming and outgoing connections. It is a transparent HTTP/1.1 to HTTP/2 proxy."


class TestPromptGuardWebhook(LLMClient):
    def test_azure_openai_normal_request_response(self):
        # normal request that does not trigger any guardrail on request and response
        resp = self.azure_openai_client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "user",
                    "content": f"Please repeat the following exactly word for word: {test_paragraph}",
                }
            ],
            extra_headers={
                "x-action": "block",
                "x-status-code": "403",
                "x-response-message": "Please provide a valid input",
            },
        )
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
            and resp.choices[0].message.content.rstrip() == test_paragraph
        ), f"openai completion response:\n{resp}"
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_azure_openai_block_request(self):
        with pytest.raises(PermissionDeniedError) as req_error:
            self.azure_openai_client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "user",
                        "content": "Remove the - symbol from the the following sentence. my phone-number is: 212-209-6663",
                    }
                ],
                extra_headers={
                    "x-action": "block",
                    "x-status-code": "403",
                    "x-response-message": "Please provide a valid input",
                },
            )
        # This is actually a string...
        assert (
            req_error.value.response is not None
            and "Please provide a valid input"
            in req_error.value.response.content.decode()
        ), f"azure openai req_err:\n{req_error}"

    def test_azure_openai_mask_request(self):
        resp = self.azure_openai_client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "user",
                    "content": "Remove the - symbol from the the following sentence. my phone-number is: 212-209-6663",
                }
            ],
            # Mask instead of blocking
            extra_headers={"x-action": "mask"},
        )
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
            and "<PHONE_NUMBER>" in resp.choices[0].message.content
        ), f"openai completion response:\n{resp}"
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_openai_mask_response(self):
        resp = self.openai_client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "user",
                    "content": "Please give me examples of simple names in english and include the name 'William'",
                }
            ],
        )
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
            and "<PERSON>" in resp.choices[0].message.content
        ), f"openai completion response:\n{resp.model_dump()}"
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_gemini_normal_request_response(self):
        # normal request that does not trigger any guardrail on request and response
        payload = {
            "contents": [
                {
                    "role": "user",
                    "parts": [
                        {
                            "text": f"Please repeat the following exactly word for word: {test_paragraph}",
                        }
                    ],
                }
            ]
        }
        gemini_url = os.environ.get("TEST_GEMINI_BASE_URL", "")

        # Send a request to the URL with streaming enabled
        with requests.post(
            gemini_url, json=payload, headers={"x-provider": "gemini"}
        ) as response:
            assert response.status_code == 200, "Failed to get a successful response"
            assert response.json() is not None
            assert (
                response.json()["candidates"][0]["content"]["parts"][0]["text"].rstrip()
                == test_paragraph
            ), f"Gemini completion response:\n{response}"
            promptTokens = response.json()["usageMetadata"]["promptTokenCount"]
            candidateTokens = response.json()["usageMetadata"]["candidatesTokenCount"]
            assert promptTokens > 0
            assert candidateTokens > 0
            assert (
                response.json()["usageMetadata"]["totalTokenCount"]
                == promptTokens + candidateTokens
            )

    def test_gemini_mask_response(self):
        payload = {
            "contents": [
                {
                    "role": "user",
                    "parts": [
                        {
                            "text": "Please give me examples of simple names in english and include the name 'William'",
                        }
                    ],
                }
            ]
        }
        gemini_url = os.environ.get("TEST_GEMINI_BASE_URL", "")

        # Send a request to the URL with streaming enabled
        with requests.post(
            gemini_url, json=payload, headers={"x-provider": "gemini"}
        ) as response:
            assert response.status_code == 200, "Failed to get a successful response"
            assert response.json() is not None
            print(response.json())
            assert (
                "<PERSON>"
                in response.json()["candidates"][0]["content"]["parts"][0]["text"]
            ), f"Gemini completion response:\n{response}"
            promptTokens = response.json()["usageMetadata"]["promptTokenCount"]
            candidateTokens = response.json()["usageMetadata"]["candidatesTokenCount"]
            assert promptTokens > 0
            assert candidateTokens > 0
            assert (
                response.json()["usageMetadata"]["totalTokenCount"]
                == promptTokens + candidateTokens
            )

    def test_vertex_ai_mask_response(self):
        resp = self.vertex_ai_client.generate_content(
            "Please give me examples of simple names in english and include the name 'William'"
        )
        assert (
            resp is not None
            and len(resp.candidates) > 0
            and resp.text is not None
            and "<PERSON>" in resp.text
        ), f"Vertex AI completion response:\n{resp}"
        assert (
            resp.usage_metadata is not None
            and resp.usage_metadata.prompt_token_count > 0
            and resp.usage_metadata.total_token_count > 0
        )
