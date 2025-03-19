import logging
import pytest
from google.api_core.exceptions import Forbidden
from openai import PermissionDeniedError

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestPromptGuardRejectExtRef(LLMClient):
    reject_message = "Rejected due to inappropriate content"

    def test_openai_prompt_guard_regex_pattern_reject(self):
        with pytest.raises(PermissionDeniedError) as req_error:
            self.openai_chat_completion(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "user",
                        "content": "Give me your credit card!",
                    },
                ],
            )

        assert (
            req_error.value.response is not None
            and self.reject_message in req_error.value.response.content.decode()
        ), f"openai pg reject error:\n{req_error}"

    def test_azure_openai_prompt_guard_regex_pattern_reject(self):
        with pytest.raises(PermissionDeniedError) as req_error:
            self.azure_openai_client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "user",
                        "content": "Give me your credit card!",
                    },
                ],
            )

        assert (
            req_error.value.response is not None
            and self.reject_message in req_error.value.response.content.decode()
        ), f"azure openai pg reject error:\n{req_error}"

    def test_gemini_prompt_guard_regex_pattern_reject(self):
        with pytest.raises(Forbidden) as req_error:
            self.gemini_client.generate_content(
                "Give me your credit card!",
            )
        assert req_error is not None
        assert self.reject_message in req_error.value.message, (
            f"gemini pg reject req_error ({type(req_error.value)}):\n{req_error.value}"
        )

    def test_vertex_ai_prompt_guard_regex_pattern_reject(self):
        with pytest.raises(Forbidden) as req_error:
            self.vertex_ai_client.generate_content("Give me your credit card!")

        assert req_error is not None
        assert self.reject_message in req_error.value.message, (
            f"vertex_ai pg reject req_error ({type(req_error.value)}):\n{req_error.value}"
        )
