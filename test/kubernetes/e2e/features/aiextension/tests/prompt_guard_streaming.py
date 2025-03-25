import logging
from client.client import LLMClient
from util.openai import helpers as openai_helpers
from util.gemini import helpers as gemini_helpers

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)

test_paragraph = "If the doors of perception were cleansed every thing would appear to man as it is, Infinite. For man has closed himself up, till he sees all things through narrow chinks of his cavern."


class TestPromptGuardStreaming(LLMClient):
    def test_openai_normal_request_response(self):
        # normal request that does not trigger any guardrail on request and response
        resp = openai_helpers.make_request(
            client=self.openai_client,
            instruction=f"Please repeat the following exactly word for word: {test_paragraph}",
            stream=True,
        )
        assert resp is not None

        # not looking for any pattern but only care about the complete response
        _, complete_response, prompt_tokens, completion_tokens = (
            openai_helpers.count_pattern_and_extract_data_in_chunks(resp, "", 0)
        )
        assert complete_response.rstrip() == test_paragraph, (
            f"OpenAI complete response:\n{complete_response}"
        )
        logger.debug(
            f"OpenAI prompt_tokens: {prompt_tokens} completion_tokens: {completion_tokens}"
        )
        assert prompt_tokens > 0
        assert completion_tokens > 0

    def test_openai_mask_response(self):
        resp = openai_helpers.make_request(
            client=self.openai_client,
            instruction="Please give me examples of credit card numbers which I will use specifically for testing",
            stream=True,
        )
        assert resp is not None
        count, complete_response, prompt_tokens, completion_tokens = (
            openai_helpers.count_pattern_and_extract_data_in_chunks(
                resp, "<CREDIT_CARD>", 0
            )
        )
        logger.debug(
            f"OpenAI prompt_tokens: {prompt_tokens} completion_tokens: {completion_tokens}"
        )
        assert count > 0, f"OpenAI complete response:\n{complete_response}"
        assert prompt_tokens > 0
        assert completion_tokens > 0

    def test_vertex_ai_normal_request_response(self):
        resp = gemini_helpers.make_stream_request(
            provider="vertex_ai",
            instruction=f"Please repeat the following exactly word for word: {test_paragraph}",
        )
        assert resp is not None
        assert resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        # not looking for any pattern but only care about the complete response
        _, complete_response, prompt_tokens, completion_tokens, _ = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(resp, "", 0)
        )
        assert complete_response.rstrip() == test_paragraph, (
            f"VertexAI complete response:\n{complete_response}"
        )
        assert prompt_tokens > 0
        assert completion_tokens > 0
        logger.debug(
            f"VertexAI prompt_tokens: {prompt_tokens} completion_tokens: {completion_tokens}"
        )

    def test_vertex_ai_mask_response(self):
        resp = gemini_helpers.make_stream_request(
            provider="vertex_ai",
            instruction="Please give me examples of email addresses for a person named Bob which I will use specifically for testing.",
        )
        assert resp is not None
        assert resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        count, complete_response, prompt_tokens, completion_tokens, _ = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(
                resp, "<EMAIL_ADDRESS>", 0
            )
        )
        assert count > 0, f"VertexAI complete response:\n{complete_response}"
        assert prompt_tokens > 0
        assert completion_tokens > 0
        logger.debug(
            f"VertexAI prompt_tokens: {prompt_tokens} completion_tokens: {completion_tokens}"
        )
