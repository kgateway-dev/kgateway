import requests
import json
import logging
import os

from typing import Tuple

logger = logging.getLogger(__name__)


def make_stream_request(
    provider: str,
    instruction: str,
    tools: list | None = None,
    addition_contents: list | None = None,
):
    payload = {
        "safetySettings": {
            "category": "HARM_CATEGORY_DANGEROUS_CONTENT",
            "threshold": "BLOCK_ONLY_HIGH",
        },
        "contents": [
            {
                "role": "user",
                "parts": [
                    {
                        "text": instruction,
                    }
                ],
            }
        ],
    }
    if tools:
        payload["tools"] = tools

    if addition_contents:
        payload["contents"] += addition_contents

    match provider:
        case "vertex_ai":
            url = os.environ.get("TEST_VERTEX_AI_BASE_URL", "")
        case "gemini":
            url = os.environ.get("TEST_GEMINI_BASE_URL", "")
        case _:
            return None

    # Send a request to the URL with streaming enabled
    return requests.post(url, json=payload, stream=True)


def count_pattern_and_extract_data_in_chunks(
    resp, pattern: str, choice_index: int = 0
) -> Tuple[int, str, int, int, dict]:
    """
    This helper search for a pattern in all the chunks from a specific choice_index
    While iterating through the chunks, it also extract other data. The return values in the tuple:
        pattern_match_count: int
        complete_response: str - concatenate all the contents for that single choice_index into a single string
        prompt_tokens: int
        completion_tokens: int
        function_call_response: dict - Do we need this as it seems that Gemini doesn't do streaming for function call
    """
    SSE_DATA_FIELD_NAME: str = "data:"
    count = 0
    complete_response = ""
    function_response = {}
    prompt_tokens = 0
    completion_tokens = 0

    logger.debug(f"count_pattern_and_extract_data_in_chunks(): pattern: {pattern}")
    for chunk in resp.iter_lines(decode_unicode=True):
        if chunk:  # Skip empty lines
            logger.debug(f"count_pattern_and_extract_data_in_chunks(): chunk: {chunk}")
            if not chunk.startswith(SSE_DATA_FIELD_NAME):
                continue
            try:
                data = json.loads(chunk[len(SSE_DATA_FIELD_NAME) :])
                assert "candidates" in data, "Missing 'candidates' key in the data"
                assert len(data["candidates"]) > 0, "'candidates' should not be empty"

                # Verify that each chunk has reasonable content
                for i, candidate in enumerate(data["candidates"]):
                    if i != choice_index:
                        continue
                    assert "content" in candidate, "Missing 'content' in candidate"
                    content = candidate["content"]
                    if (
                        candidate.get("finishReason", None) is not None
                        and "parts" not in content
                    ):
                        continue
                    assert "parts" in content, f"Missing 'parts' in content: {data}"
                    assert len(content["parts"]) > 0, "'parts' should not be empty"
                    for part in content["parts"]:
                        if "functionCall" in part:
                            function_response = part["functionCall"]
                        if "text" in part:
                            assert isinstance(part["text"], str), (
                                "Expected 'text' to be a string"
                            )
                            complete_response += part["text"]
                            if pattern in part["text"]:
                                count += 1
                if "usageMetadata" in data:
                    # for Gemini, the tokens are split into each chunk but the prompt tokens
                    # are repeated, so it's `=` and not `+=`
                    prompt_tokens = data["usageMetadata"].get("promptTokenCount", 0)
                    completion_tokens += data["usageMetadata"].get(
                        "candidatesTokenCount", 0
                    )

            except ValueError as e:
                logger.error(f"invalid json: {e}")
                return 0, "", 0, 0, {}

    return count, complete_response, prompt_tokens, completion_tokens, function_response
