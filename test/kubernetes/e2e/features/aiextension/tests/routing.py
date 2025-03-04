import logging
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
        logger.debug(f"openai completion response:\n{resp}")
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
        logger.debug(f"openai completion response:\n{resp}")
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

    def test_mistralai_completion(self):
        resp = self.mistral_client.chat.complete(
            model="mistral-small-latest",
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
        logger.debug(f"mistralai completion response:\n{resp}")
        assert (
            resp is not None
            and resp.choices is not None
            and len(resp.choices) > 0
            and resp.choices[0].message.content != ""
        )
        assert resp.usage.prompt_tokens > 0
        assert (
            resp.usage.completion_tokens is not None
            and resp.usage.completion_tokens > 0
        )

    def test_anthropic_completion(self):
        resp = self.anthropic_client.messages.create(
            max_tokens=1024,
            model="claude-3-haiku-20240307",
            system="You are a poetic assistant, skilled in explaining complex programming concepts with creative flair.",
            messages=[
                {
                    "role": "user",
                    "content": "Compose a poem that explains the concept of recursion in programming.",
                },
            ],
        )
        assert resp is not None
        logger.debug(f"anthropic completion response:\n{resp}")
        assert len(resp.content) > 0
        assert resp.usage.output_tokens > 0

    def test_gemini_completion(self):
        resp = self.gemini_client.generate_content(
            "Compose a poem that explains the concept of recursion in programming."
        )
        assert resp is not None
        logger.debug(f"gemini completion response:\n{resp}")
        assert len(resp.candidates) == 1
        assert resp.candidates[0].finish_reason == GeminiFinishReason.STOP
        assert resp.usage_metadata.prompt_token_count > 0

    def test_vertex_ai_completion(self):
        resp = self.vertex_ai_client.generate_content(
            "Compose a poem that explains the concept of recursion in programming."
        )
        assert resp is not None
        logger.debug(f"Vertex AI completion response:\n{resp}")
        assert len(resp.candidates) == 1
        assert resp.candidates[0].finish_reason == VertexFinishReason.STOP
        assert resp.usage_metadata.prompt_token_count > 0
