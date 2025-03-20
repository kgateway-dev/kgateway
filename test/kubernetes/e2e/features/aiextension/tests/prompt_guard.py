import logging
import pytest
from openai import BadRequestError

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestPromptGuard(LLMClient):
    def test_openai_mask_response(self):
        resp = self.openai_client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "user",
                    "content": "Please give me examples of credit card numbers which I will use specifically for testing",
                }
            ],
        )
        assert (
            resp is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content is not None
            and "<CREDIT_CARD>" in resp.choices[0].message.content
        ), f"openai completion response:\n{resp.model_dump()}"
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_openai_block_request_regex(self):
        with pytest.raises(BadRequestError) as req_error:
            self.openai_client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "user",
                        "content": "Remove the - symbol from the following sentence. my phone-number is: 212-209-6663",
                    }
                ],
            )
        # This is actually a string...
        assert (
            req_error.value.response is not None
            and "Please provide a valid input"
            in req_error.value.response.content.decode()
        ), f"req_error:\n{req_error}"

    def test_vertex_ai_mask_response(self):
        resp = self.vertex_ai_client.generate_content(
            "Please give me examples of email addresses for a person named Bob which I will use specifically for testing."
        )
        assert (
            resp is not None
            and len(resp.candidates) > 0
            and resp.text is not None
            and "<EMAIL_ADDRESS>" in resp.text
        ), f"Vertex AI completion response:\n{resp.text}"
        assert (
            resp.usage_metadata is not None
            and resp.usage_metadata.prompt_token_count > 0
            and resp.usage_metadata.total_token_count > 0
        )
