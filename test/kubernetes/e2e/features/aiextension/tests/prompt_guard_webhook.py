import logging
import pytest
from openai import BadRequestError

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestPromptGuard(LLMClient):
    def test_webhook_regex_request(self):
        with pytest.raises(BadRequestError) as req_error:
            self.openai_client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "user",
                        "content": "my facebook password is: 110",
                    }
                ],
            )
        # This is actually a string...
        assert (
            req_error.value.response is not None
            and "don't send your really facebook password to LLM provider"
            in req_error.value.response.content.decode()
        ), f"req_error:\n{req_error}"
        
    def test_webhook_mask_response(self):
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
    def test_webhook_pass_response(self):
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
        )
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
