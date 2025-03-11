import copy
import json

from ext_proc.provider import Tokens, TokensDetails, Anthropic, Gemini, OpenAI
from guardrails import api as webhook_api
from ext_proc.streamchunkdata import StreamChunkDataType
from typing import Dict, Any


def test_tokens_addition():
    tokens1 = Tokens(completion=5, prompt=10)
    tokens2 = Tokens(completion=3, prompt=7)
    result = tokens1 + tokens2
    assert result.completion == 8
    assert result.prompt == 17
    assert result.prompt_details is None
    assert result.completion_details is None

    token_details = TokensDetails(
        cached=1,
        tool_used=2,
        accepted_prediction=3,
        rejected_prediction=4,
        reasoning=5,
        text=6,
        audio=7,
        document=8,
        image=9,
        video=10,
    )

    # when one of the details is None, just take the other one
    tokens1 = Tokens(completion=5, prompt=10, prompt_details=token_details)
    tokens2 = Tokens(completion=3, prompt=7, completion_details=token_details)
    result = tokens1 + tokens2
    assert result.prompt_details == token_details
    assert result.completion_details == token_details

    # when both details exist in each token, they are added together
    expected_token_details = TokensDetails(
        cached=2,
        tool_used=4,
        accepted_prediction=6,
        rejected_prediction=8,
        reasoning=10,
        text=12,
        audio=14,
        document=16,
        image=18,
        video=20,
    )
    tokens1 = Tokens(
        completion=5,
        prompt=10,
        prompt_details=token_details,
        completion_details=token_details,
    )
    tokens2 = Tokens(
        completion=3,
        prompt=7,
        prompt_details=token_details,
        completion_details=token_details,
    )
    result = tokens1 + tokens2
    assert result.prompt_details == expected_token_details
    assert result.completion_details == expected_token_details


def test_tokens_total_tokens():
    tokens = Tokens(completion=5, prompt=10)
    assert tokens.total_tokens() == 15


def anthropic_req() -> dict:
    return {
        "model": "claude-3-5-sonnet-20241022",
        "max_tokens": 1024,
        "messages": [{"role": "user", "content": "Hello, world"}],
        "stream": True,
    }


def anthropic_resp() -> dict:
    return {
        "id": "msg_01EgaC99fAqgC1sudjBwhTvn",
        "type": "message",
        "role": "assistant",
        "model": "claude-3-5-sonnet-20241022",
        "content": [{"type": "text", "text": "Hi! How can I help you today?"}],
        "stop_reason": "end_turn",
        "usage": {"input_tokens": 10, "output_tokens": 12},
    }


def anthropic_stream_resp_message_start() -> Dict[str, Any]:
    return json.loads(
        '{"type":"message_start","message":{"id":"msg_014p7gG3wDgGV9EUtLvnow3U","type":"message","role":"assistant","model":"claude-3-haiku-20240307","stop_sequence":null,"usage":{"input_tokens":472,"output_tokens":2},"content":[],"stop_reason":null}}'
    )


def anthropic_stream_resp_start() -> Dict[str, Any]:
    return json.loads(
        '{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}'
    )


def anthropic_stream_resp_second() -> Dict[str, Any]:
    return json.loads(
        '{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}'
    )


def anthropic_stream_resp_last() -> Dict[str, Any]:
    return json.loads('{"type": "content_block_stop", "index": 0}')


def test_anthropic_tokens():
    provider = Anthropic()
    tokens = provider.tokens(anthropic_resp())
    assert tokens.completion == 12
    assert tokens.prompt == 10


def test_anthropic_get_model_req():
    provider = Anthropic()
    headers_jsn = {}
    assert (
        provider.get_model_req(anthropic_req(), headers_jsn)
        == "claude-3-5-sonnet-20241022"
    )


def test_anthropic_get_model_resp():
    provider = Anthropic()
    assert provider.get_model_resp(anthropic_resp()) == "claude-3-5-sonnet-20241022"


def test_anthropic_is_streaming_req():
    provider = Anthropic()
    headers_jsn = {}
    assert provider.is_streaming_req(anthropic_req(), headers_jsn) is True


def test_anthropic_get_num_tokens_from_body():
    provider = Anthropic()
    num_tokens = provider.get_num_tokens_from_body(anthropic_req())
    assert num_tokens == 12


def test_anthropic_iterate_str_req_messages():
    provider = Anthropic()

    def callback(role, content):
        return content.upper()

    req = anthropic_req()
    provider.iterate_str_req_messages(req, callback)
    assert req["messages"][0]["content"] == "HELLO, WORLD"


def test_anthropic_iterate_str_resp_messages():
    provider = Anthropic()

    def callback(role, content):
        return content.upper()

    resp = anthropic_resp()
    provider.iterate_str_resp_messages(resp, callback)
    assert resp["content"][0]["text"] == "HI! HOW CAN I HELP YOU TODAY?"


def test_anthropic_all_req_content():
    provider = Anthropic()
    content = provider.all_req_content(anthropic_req())
    print(content)
    expected_content = "role: user:\nHello, world"
    assert content == expected_content


def test_anthropic_construct_response_webhook_request_body():
    provider = Anthropic()
    body = anthropic_resp()
    responseChoices = provider.construct_response_webhook_request_body(body)
    assert responseChoices.choices[0].message.role == body["role"]
    assert responseChoices.choices[0].message.content == body["content"][0]["text"]


def test_anthropic_update_response_body_from_webhook():
    provider = Anthropic()
    body = anthropic_resp()
    test_content = "There is no road; You make your own path as you walk."
    modified = provider.construct_response_webhook_request_body(body)
    modified.choices[0].message.content = test_content
    provider.update_response_body_from_webhook(body, modified)
    original_body = anthropic_resp()
    assert body != original_body
    original_body["content"][0]["text"] = test_content
    assert body == original_body

    # role doesn't get updated
    original_role = original_body["role"]
    modified.choices[0].message.role = "ai"
    provider.update_response_body_from_webhook(body, modified)
    assert body == original_body
    assert body["role"] == original_role


def test_anthropic_extract_contents_from_resp_chunk():
    provider = Anthropic()
    jsn = anthropic_stream_resp_start()
    assert provider.extract_contents_from_resp_chunk(jsn) == [b""]
    jsn = anthropic_stream_resp_second()
    assert provider.extract_contents_from_resp_chunk(jsn) == [b"Hello"]
    jsn = anthropic_stream_resp_last()
    assert provider.extract_contents_from_resp_chunk(jsn) is None


def test_anthropic_update_stream_resp_contents():
    provider = Anthropic()
    jsn = anthropic_stream_resp_start()
    expected = b"How are you?"
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) == [expected]

    jsn = anthropic_stream_resp_second()
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) == [expected]

    jsn = anthropic_stream_resp_last()
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) is None


def gemini_req() -> dict:
    return {
        "contents": [{"role": "user", "parts": [{"text": "explain yourself mr.ai"}]}]
    }


def gemini_resp() -> dict:
    return {
        "candidates": [
            {
                "content": {
                    "role": "model",
                    "parts": [
                        {
                            "text": "I am a large language model, also known as a conversational AI or chatbot."
                        }
                    ],
                },
            }
        ],
        "usageMetadata": {
            "promptTokenCount": 5,
            "candidatesTokenCount": 241,
            "totalTokenCount": 246,
        },
        "modelVersion": "gemini-1.5-flash-001",
    }


def gemini_multi_choices_resp() -> dict:
    return {
        "candidates": [
            {
                "content": {
                    "role": "model",
                    "parts": [
                        {
                            "text": "I am a large language model, also known as a conversational AI or chatbot."
                        }
                    ],
                },
            },
            {
                "content": {"role": "model", "parts": [{"text": "I am a grok."}]},
            },
        ],
        "usageMetadata": {
            "promptTokenCount": 5,
            "candidatesTokenCount": 341,
            "totalTokenCount": 346,
        },
        "modelVersion": "gemini-1.5-flash-001",
    }


def gemini_stream_resp_first() -> Dict[str, Any]:
    return json.loads(
        '{"candidates": [{"content": {"parts": [{"text": "Envoy is a"}],"role": "model"},"index": 0,"safetyRatings": [{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HATE_SPEECH","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HARASSMENT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_DANGEROUS_CONTENT","probability": "NEGLIGIBLE"}]}],"usageMetadata": {"promptTokenCount": 76,"candidatesTokenCount": 4,"totalTokenCount": 80},"modelVersion": "gemini-1.5-flash-001"}'
    )


def gemini_stream_resp_last() -> Dict[str, Any]:
    return json.loads(
        '{"candidates": [{"content": {"parts": [{"text": "Note:** This is just a small sampling of simple names. There are many other beautiful and unique names that could be considered. The best name is the one that you love the most!"}],"role": "model"},"finishReason": "STOP","index": 0,"safetyRatings": [{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HATE_SPEECH","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HARASSMENT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_DANGEROUS_CONTENT","probability": "NEGLIGIBLE"}]}],"usageMetadata": {"promptTokenCount": 10,"candidatesTokenCount": 368,"totalTokenCount": 378},"modelVersion": "gemini-1.5-flash-001"}'
    )


def gemini_stream_resp_no_usage() -> Dict[str, Any]:
    return json.loads(
        '{"candidates": [{"content": {"role": "model", "parts": [{"text": "These examples should give you a good starting point for creating test email addresses"}]}, "safetyRatings": [{"category": "HARM_CATEGORY_HATE_SPEECH", "probability": "NEGLIGIBLE", "probabilityScore": 0.05834961, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.09814453}, {"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "probability": "NEGLIGIBLE", "probabilityScore": 0.13671875, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.091308594}, {"category": "HARM_CATEGORY_HARASSMENT", "probability": "NEGLIGIBLE", "probabilityScore": 0.10107422, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.026733398}, {"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "probability": "NEGLIGIBLE", "probabilityScore": 0.04736328, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.05102539}]}], "modelVersion": "gemini-1.5-flash-001", "createTime": "2025-02-26T17:16:05.905870Z", "responseId": "VUy_Z46lN-a22PgPorGBuQY"}'
    )


def gemini_stream_resp_with_usage() -> Dict[str, Any]:
    return json.loads(
        '{"candidates": [{"content": {"role": "model", "parts": [{"text": "These examples should give you a good starting point for creating test email addresses"}]}, "safetyRatings": [{"category": "HARM_CATEGORY_HATE_SPEECH", "probability": "NEGLIGIBLE", "probabilityScore": 0.05834961, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.09814453}, {"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "probability": "NEGLIGIBLE", "probabilityScore": 0.13671875, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.091308594}, {"category": "HARM_CATEGORY_HARASSMENT", "probability": "NEGLIGIBLE", "probabilityScore": 0.10107422, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.026733398}, {"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "probability": "NEGLIGIBLE", "probabilityScore": 0.04736328, "severity": "HARM_SEVERITY_NEGLIGIBLE", "severityScore": 0.05102539}]}], "usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 283,"totalTokenCount": 303,"promptTokensDetails": [{"modality": "TEXT","tokenCount": 20}],"candidatesTokensDetails": [{"modality": "TEXT","tokenCount": 283}]}, "modelVersion": "gemini-1.5-flash-001", "createTime": "2025-02-26T17:16:05.905870Z", "responseId": "VUy_Z46lN-a22PgPorGBuQY"}'
    )


def test_gemini_tokens():
    provider = Gemini()
    tokens = provider.tokens(gemini_resp())
    assert tokens.completion == 241
    assert tokens.prompt == 5
    assert tokens.completion_details is None
    assert tokens.prompt_details is None

    jsn = gemini_stream_resp_with_usage()
    # "usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 283,"totalTokenCount": 303,"promptTokensDetails": [{"modality": "TEXT","tokenCount": 20}],"candidatesTokensDetails": [{"modality": "TEXT","tokenCount": 283}]}
    tokens = provider.tokens(jsn)
    assert tokens.prompt == 20
    assert tokens.completion == 283
    assert tokens.completion_details is not None
    assert tokens.prompt_details is not None
    assert tokens.prompt_details.text == 20
    assert tokens.prompt_details.audio == 0
    assert tokens.prompt_details.video == 0
    assert tokens.prompt_details.image == 0
    assert tokens.prompt_details.document == 0
    assert tokens.completion_details.text == 283
    assert tokens.completion_details.audio == 0
    assert tokens.completion_details.video == 0
    assert tokens.completion_details.image == 0
    assert tokens.completion_details.document == 0


def test_gemini_get_model_req():
    provider = Gemini()
    body_jsn = {}
    headers_jsn = {"x-llm-model": "test-model"}
    assert provider.get_model_req(body_jsn, headers_jsn) == "test-model"


def test_gemini_get_model_resp():
    provider = Gemini()
    assert provider.get_model_resp(gemini_resp()) == "gemini-1.5-flash-001"


def test_gemini_is_streaming_req():
    provider = Gemini()
    body_jsn = {}
    headers_jsn = {"x-chat-streaming": "true"}
    assert provider.is_streaming_req(body_jsn, headers_jsn) is True


def test_gemini_get_num_tokens_from_body():
    provider = Gemini()
    body = gemini_req()
    num_tokens = provider.get_num_tokens_from_body(body)
    assert num_tokens == 10


def test_gemini_iterate_str_req_messages():
    provider = Gemini()
    body = gemini_req()

    def callback(role, content):
        return content.upper()

    provider.iterate_str_req_messages(body, callback)
    assert body["contents"][0]["parts"][0]["text"] == "EXPLAIN YOURSELF MR.AI"


def test_gemini_iterate_str_resp_messages():
    provider = Gemini()
    body = gemini_resp()

    def callback(role, content):
        return content.upper()

    provider.iterate_str_resp_messages(body, callback)
    assert (
        body["candidates"][0]["content"]["parts"][0]["text"]
        == "I AM A LARGE LANGUAGE MODEL, ALSO KNOWN AS A CONVERSATIONAL AI OR CHATBOT."
    )


def test_gemini_all_req_content():
    provider = Gemini()
    body = gemini_req()
    content = provider.all_req_content(body)
    expected_content = "role: user:\nexplain yourself mr.ai\n"
    assert content == expected_content


def test_gemini_construct_request_webhook_request_body():
    provider = Gemini()
    body = gemini_req()
    promptMessages = provider.construct_request_webhook_request_body(body)
    original_body = gemini_req()
    expected = webhook_api.PromptMessages()
    expected.messages.append(
        webhook_api.Message(
            role=original_body["contents"][0]["role"],
            content=original_body["contents"][0]["parts"][0]["text"],
        )
    )
    assert promptMessages == expected


def test_gemini_update_request_body_from_webhook():
    provider = Gemini()
    body = gemini_req()
    test_content = "Write a haiku that explains the concept of inception."
    modified = provider.construct_request_webhook_request_body(body)
    modified.messages[0].content = test_content
    provider.update_request_body_from_webhook(body, modified)
    original_body = gemini_req()
    assert body != original_body
    original_body["contents"][0]["parts"][0]["text"] = test_content
    assert body == original_body

    # roles cannot be changed
    new_prompts = copy.deepcopy(modified)
    new_prompts.messages[0].role = "me"
    provider.update_request_body_from_webhook(body, new_prompts)
    # the role fields are ignore, so the result is still the same as the modified "original_body"
    assert body == original_body


def test_gemini_construct_response_webhook_request_body():
    provider = Gemini()
    body = gemini_resp()
    responseChoices = provider.construct_response_webhook_request_body(body)
    assert (
        responseChoices.choices[0].message.role
        == body["candidates"][0]["content"]["role"]
    )
    assert (
        responseChoices.choices[0].message.content
        == body["candidates"][0]["content"]["parts"][0]["text"]
    )

    body = gemini_multi_choices_resp()
    responseChoices = provider.construct_response_webhook_request_body(body)
    for i, choice in enumerate(responseChoices.choices):
        assert choice.message.role == body["candidates"][i]["content"]["role"]
        assert (
            choice.message.content
            == body["candidates"][i]["content"]["parts"][0]["text"]
        )


def test_gemini_update_response_body_from_webhook():
    provider = Gemini()
    body = gemini_resp()
    test_content = "I am no body!"
    test_content2 = "I am who I am!"
    modified = provider.construct_response_webhook_request_body(body)
    modified.choices[0].message.content = test_content
    provider.update_response_body_from_webhook(body, modified)
    original_body = gemini_resp()
    assert body != original_body
    # make sure only content is changed and everything else remain the same
    original_body["candidates"][0]["content"]["parts"][0]["text"] = test_content
    assert body == original_body

    # multi choices response
    body = gemini_multi_choices_resp()
    expected = provider.construct_response_webhook_request_body(body)
    expected.choices[0].message.content = test_content2
    expected.choices[1].message.content = test_content
    provider.update_response_body_from_webhook(body, expected)
    original_body = gemini_multi_choices_resp()
    assert body != original_body
    # make sure only content is changed and everything else remain the same
    original_body["candidates"][0]["content"]["parts"][0]["text"] = test_content2
    original_body["candidates"][1]["content"]["parts"][0]["text"] = test_content
    assert body == original_body

    # role doesn't get updated
    body = gemini_resp()
    expected = provider.construct_response_webhook_request_body(body)
    original_role = expected.choices[0].message.role
    expected.choices[0].message.role = "ai"
    expected.choices[0].message.content = test_content
    provider.update_response_body_from_webhook(body, expected)
    original_body = gemini_resp()
    assert body != original_body
    # make sure only content is changed and everything else remain the same
    original_body["candidates"][0]["content"]["parts"][0]["text"] = test_content
    assert body == original_body
    assert body["candidates"][0]["content"]["role"] == original_role


def test_gemini_get_stream_resp_chunk_type():
    provider = Gemini()
    jsn = gemini_stream_resp_first()
    assert provider.get_stream_resp_chunk_type(jsn) == StreamChunkDataType.NORMAL_TEXT
    assert (
        provider.get_stream_resp_chunk_type(gemini_stream_resp_last())
        == StreamChunkDataType.FINISH
    )


def test_gemini_extract_contents_from_resp_chunk():
    provider = Gemini()
    jsn = gemini_stream_resp_first()
    assert provider.extract_contents_from_resp_chunk(jsn) == [b"Envoy is a"]
    jsn = gemini_stream_resp_last()
    assert provider.extract_contents_from_resp_chunk(jsn) == [
        b"Note:** This is just a small sampling of simple names. There are many other beautiful and unique names that could be considered. The best name is the one that you love the most!"
    ]


def test_gemini_update_stream_resp_contents():
    provider = Gemini()
    jsn = gemini_stream_resp_first()
    expected = b"How are you?"
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) == [expected]

    jsn = gemini_stream_resp_last()
    expected = b"What can I help you?"
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) == [expected]


def test_gemini_update_stream_resp_usage_token():
    provider = Gemini()
    # Test that we will create the usageMetadata field if it's missing
    jsn = gemini_stream_resp_no_usage()
    promptTokenCount = 21
    completionTokenCount = 123
    total = promptTokenCount + completionTokenCount
    tokens = Tokens(completion=completionTokenCount, prompt=promptTokenCount)
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usageMetadata"]["promptTokenCount"] == promptTokenCount
    assert jsn["usageMetadata"]["candidatesTokenCount"] == completionTokenCount
    assert jsn["usageMetadata"]["totalTokenCount"] == total
    assert jsn["usageMetadata"].get("promptTokensDetails") is None
    assert jsn["usageMetadata"].get("candidatesTokensDetails") is None

    # Test that we will update the usageMetadata field if it already exists
    jsn = gemini_stream_resp_with_usage()
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usageMetadata"]["promptTokenCount"] == promptTokenCount
    assert jsn["usageMetadata"]["candidatesTokenCount"] == completionTokenCount
    assert jsn["usageMetadata"]["totalTokenCount"] == total
    assert jsn["usageMetadata"].get("promptTokensDetails") is None
    assert jsn["usageMetadata"].get("candidatesTokensDetails") is None

    # Test that we will update the usageMetadata with tokens details
    tokens.completion_details = TokensDetails(text=123)
    tokens.prompt_details = TokensDetails(text=456)
    jsn = gemini_stream_resp_with_usage()
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usageMetadata"]["promptTokenCount"] == promptTokenCount
    assert jsn["usageMetadata"]["candidatesTokenCount"] == completionTokenCount
    assert jsn["usageMetadata"]["totalTokenCount"] == total

    details = jsn["usageMetadata"].get("promptTokensDetails")
    assert details is not None
    assert len(details) == 1
    assert details[0]["modality"] == "TEXT"
    assert details[0]["tokenCount"] == 456

    details = jsn["usageMetadata"].get("candidatesTokensDetails")
    assert details is not None
    assert len(details) == 1
    assert details[0]["modality"] == "TEXT"
    assert details[0]["tokenCount"] == 123


def openai_req() -> dict:
    return {
        "model": "gpt-4o-mini",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {
                "role": "user",
                "content": "Write a haiku that explains the concept of recursion.",
            },
        ],
        "stream": True,
    }


def openai_resp() -> dict:
    return {
        "object": "chat.completion",
        "model": "gpt-4o-mini-2024-07-18",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "Nested paths unfold,  \nEchoing steps of the past,  \nSolutions within.",
                },
                "finish_reason": "stop",
            }
        ],
        "usage": {
            "prompt_tokens": 28,
            "completion_tokens": 16,
            "total_tokens": 44,
        },
    }


def openai_multi_choices_resp() -> dict:
    return {
        "object": "chat.completion",
        "model": "gpt-4o-mini-2024-07-18",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "Nested paths unfold,  \nEchoing steps of the past,  \nSolutions within.",
                },
                "finish_reason": "stop",
            },
            {
                "index": 1,
                "message": {
                    "role": "assistant",
                    "content": "Sorry, I am lost",
                },
                "finish_reason": "stop",
            },
        ],
        "usage": {
            "prompt_tokens": 28,
            "completion_tokens": 32,
            "total_tokens": 60,
        },
    }


def openai_stream_resp_first() -> Dict[str, Any]:
    return json.loads(
        '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!","refusal":null},"logprobs":null,"finish_reason":null}]}'
    )


def openai_stream_resp_last() -> Dict[str, Any]:
    return json.loads(
        '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}'
    )


def openai_stream_resp_no_usage() -> Dict[str, Any]:
    return json.loads(
        '{"choices":[{"content_filter_results":{},"delta":{"content":"","refusal":null,"role":"assistant"},"finish_reason":null,"index":0,"logprobs":null}],"created":1740590155,"id":"chatcmpl-B5FIhX6e00wpolDWO1wswKjNXoabt","model":"gpt-4o-mini-2024-07-18","object":"chat.completion.chunk","system_fingerprint":"fp_b705f0c291","usage":null}'
    )


def openai_stream_resp_with_usage() -> Dict[str, Any]:
    return json.loads(
        '{"choices":[],"created":1740590155,"id":"chatcmpl-B5FIhX6e00wpolDWO1wswKjNXoabt","model":"gpt-4o-mini-2024-07-18","object":"chat.completion.chunk","system_fingerprint":"fp_b705f0c291","usage":{"completion_tokens":68,"completion_tokens_details":{"accepted_prediction_tokens":0,"audio_tokens":0,"reasoning_tokens":0,"rejected_prediction_tokens":0},"prompt_tokens":84,"prompt_tokens_details":{"audio_tokens":0,"cached_tokens":0},"total_tokens":152}}'
    )


def test_openai_tokens():
    provider = OpenAI()
    tokens = provider.tokens(openai_resp())
    assert tokens.completion == 16
    assert tokens.prompt == 28
    assert tokens.prompt_details is None
    assert tokens.completion_details is None

    jsn = openai_stream_resp_with_usage()
    jsn["usage"] = {
        "completion_tokens": 68,
        "completion_tokens_details": {
            "accepted_prediction_tokens": 3,
            "audio_tokens": 4,
            "reasoning_tokens": 5,
            "rejected_prediction_tokens": 6,
        },
        "prompt_tokens": 84,
        "prompt_tokens_details": {"audio_tokens": 7, "cached_tokens": 8},
        "total_tokens": 152,
    }
    tokens = provider.tokens(jsn)
    assert tokens.completion == 68
    assert tokens.completion_details is not None
    assert tokens.completion_details.accepted_prediction == 3
    assert tokens.completion_details.audio == 4
    assert tokens.completion_details.reasoning == 5
    assert tokens.completion_details.rejected_prediction == 6
    assert tokens.completion_details.cached == 0
    assert tokens.prompt == 84
    assert tokens.prompt_details is not None
    assert tokens.prompt_details.audio == 7
    assert tokens.prompt_details.cached == 8
    assert tokens.prompt_details.accepted_prediction == 0
    assert tokens.prompt_details.reasoning == 0
    assert tokens.prompt_details.rejected_prediction == 0


def test_openai_get_model_req():
    provider = OpenAI()
    headers_jsn = {}
    assert provider.get_model_req(openai_req(), headers_jsn) == "gpt-4o-mini"


def test_openai_get_model_resp():
    provider = OpenAI()
    assert provider.get_model_resp(openai_resp()) == "gpt-4o-mini-2024-07-18"


def test_openai_is_streaming_req():
    provider = OpenAI()
    headers_jsn = {}
    assert provider.is_streaming_req(openai_req(), headers_jsn) is True


def test_openai_get_num_tokens_from_body():
    provider = OpenAI()
    num_tokens = provider.get_num_tokens_from_body(openai_req())
    assert num_tokens == 32


def test_openai_iterate_str_req_messages():
    provider = OpenAI()
    body = openai_req()

    def callback(role, content):
        return content.upper()

    provider.iterate_str_req_messages(body, callback)
    assert body["messages"][0]["content"] == "YOU ARE A HELPFUL ASSISTANT."
    assert (
        body["messages"][1]["content"]
        == "WRITE A HAIKU THAT EXPLAINS THE CONCEPT OF RECURSION."
    )


def test_openai_iterate_str_resp_messages():
    provider = OpenAI()
    body = openai_resp()

    def callback(role, content):
        return content.upper()

    provider.iterate_str_resp_messages(body, callback)
    assert (
        body["choices"][0]["message"]["content"]
        == "NESTED PATHS UNFOLD,  \nECHOING STEPS OF THE PAST,  \nSOLUTIONS WITHIN."
    )


def test_openai_all_req_content():
    provider = OpenAI()
    body = openai_req()
    content = provider.all_req_content(body)
    expected_content = "role: system:\nYou are a helpful assistant.\nrole: user:\nWrite a haiku that explains the concept of recursion."
    assert content == expected_content


def test_openai_construct_request_webhook_request_body():
    provider = OpenAI()
    body = openai_req()
    promptMessages = provider.construct_request_webhook_request_body(body)
    expected = webhook_api.PromptMessages.model_validate_json(json.dumps(body))
    assert promptMessages == expected


def test_openai_update_request_body_from_webhook():
    provider = OpenAI()
    body = openai_req()
    expected = provider.construct_request_webhook_request_body(body)
    expected.messages[0].content = "You are NOT a helpful assistant."
    expected.messages[
        1
    ].content = "Write a haiku that explains the concept of inception."
    provider.update_request_body_from_webhook(body, expected)
    result = webhook_api.PromptMessages.model_validate_json(json.dumps(body))
    assert result == expected

    # roles cannot be changed
    new_prompts = copy.deepcopy(expected)
    new_prompts.messages[0].role = "ai"
    new_prompts.messages[1].role = "me"
    provider.update_request_body_from_webhook(body, new_prompts)
    result = webhook_api.PromptMessages.model_validate_json(json.dumps(body))
    # the role fields are ignore, so the result is still the same as "expected" and not "new_prompts"
    assert result == expected


def test_openai_construct_response_webhook_request_body():
    provider = OpenAI()
    body = openai_resp()
    choices = provider.construct_response_webhook_request_body(body)
    expected = webhook_api.ResponseChoices.model_validate_json(json.dumps(body))
    assert choices == expected

    body = openai_multi_choices_resp()
    choices = provider.construct_response_webhook_request_body(body)
    expected = webhook_api.ResponseChoices.model_validate_json(json.dumps(body))
    assert choices == expected


def test_openai_update_response_body_from_webhook():
    provider = OpenAI()
    body = openai_resp()
    expected = provider.construct_response_webhook_request_body(body)
    expected.choices[
        0
    ].message.content = "There is no road; You make your own path as you walk."
    provider.update_response_body_from_webhook(body, expected)
    result = webhook_api.ResponseChoices.model_validate_json(json.dumps(body))
    assert result == expected

    # make sure only content is changed and everything else remain the same
    original_body = openai_resp()
    original_body["choices"][0]["message"]["content"] = ""
    body["choices"][0]["message"]["content"] = ""
    assert body == original_body

    body = openai_multi_choices_resp()
    expected = provider.construct_response_webhook_request_body(body)
    expected.choices[
        0
    ].message.content = "There is no road; You make your own path as you walk."
    expected.choices[1].message.content = "Paths are Made by Walking, Not Waiting."
    provider.update_response_body_from_webhook(body, expected)
    result = webhook_api.ResponseChoices.model_validate_json(json.dumps(body))
    assert result == expected

    # make sure only content is changed and everything else remain the same
    original_body = openai_multi_choices_resp()
    original_body["choices"][0]["message"]["content"] = ""
    original_body["choices"][1]["message"]["content"] = ""
    body["choices"][0]["message"]["content"] = ""
    body["choices"][1]["message"]["content"] = ""
    assert body == original_body

    # role doesn't get updated
    body = openai_resp()
    expected = provider.construct_response_webhook_request_body(body)
    original_role = expected.choices[0].message.role
    expected.choices[0].message.role = "ai"
    expected.choices[
        0
    ].message.content = "There is no road; You make your own path as you walk."
    provider.update_response_body_from_webhook(body, expected)
    result = webhook_api.ResponseChoices.model_validate_json(json.dumps(body))
    assert result != expected
    # change the role back to the original and now it should match
    expected.choices[0].message.role = original_role
    assert result == expected


def test_openai_get_stream_resp_chunk_type():
    provider = OpenAI()
    jsn = openai_stream_resp_first()
    assert provider.get_stream_resp_chunk_type(jsn) == StreamChunkDataType.NORMAL_TEXT
    jsn["choices"][0]["finish_reason"] = "stop"
    assert provider.get_stream_resp_chunk_type(jsn) == StreamChunkDataType.FINISH
    assert (
        provider.get_stream_resp_chunk_type(openai_stream_resp_last())
        == StreamChunkDataType.FINISH_NO_CONTENT
    )


def test_openai_extract_contents_from_resp_chunk():
    provider = OpenAI()
    jsn = openai_stream_resp_first()
    assert provider.extract_contents_from_resp_chunk(jsn) == [b"Hello!"]
    jsn = openai_stream_resp_last()
    assert provider.extract_contents_from_resp_chunk(jsn) is None


def test_openai_update_stream_resp_contents():
    provider = OpenAI()
    jsn = openai_stream_resp_first()
    expected = b"How are you?"
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) == [expected]

    jsn = openai_stream_resp_last()
    provider.update_stream_resp_contents(jsn, 0, expected)
    assert provider.extract_contents_from_resp_chunk(jsn) is None


def test_openai_update_stream_resp_usage_token():
    provider = OpenAI()
    # Test that we will create the usage field if it's missing
    jsn = openai_stream_resp_no_usage()
    promptTokenCount = 21
    completionTokenCount = 123
    total = promptTokenCount + completionTokenCount
    tokens = Tokens(completion=completionTokenCount, prompt=promptTokenCount)
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usage"]["prompt_tokens"] == promptTokenCount
    assert jsn["usage"]["completion_tokens"] == completionTokenCount
    assert jsn["usage"]["total_tokens"] == total
    assert jsn["usage"].get("prompt_tokens_details") is None
    assert jsn["usage"].get("completion_tokens_details") is None

    # Test that we will update the usage field if it already exists
    jsn = openai_stream_resp_with_usage()
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usage"]["prompt_tokens"] == promptTokenCount
    assert jsn["usage"]["completion_tokens"] == completionTokenCount
    assert jsn["usage"]["total_tokens"] == total
    assert jsn["usage"].get("prompt_tokens_details") is None
    assert jsn["usage"].get("completion_tokens_details") is None

    # Test that we will update the usage field with token details
    jsn = openai_stream_resp_with_usage()
    provider.update_stream_resp_usage_token(jsn, tokens)
    assert jsn["usage"]["prompt_tokens"] == promptTokenCount
    assert jsn["usage"]["completion_tokens"] == completionTokenCount
    assert jsn["usage"]["total_tokens"] == total
    assert jsn["usage"].get("prompt_tokens_details") is None
    assert jsn["usage"].get("completion_tokens_details") is None

    tokens.completion_details = TokensDetails(
        audio=1, rejected_prediction=2, accepted_prediction=3, reasoning=4
    )
    tokens.prompt_details = TokensDetails(audio=5, cached=6)
    jsn = openai_stream_resp_with_usage()
    provider.update_stream_resp_usage_token(jsn, tokens)
    details = jsn["usage"].get("prompt_tokens_details")
    assert details is not None
    assert details["audio_tokens"] == 5
    assert details["cached_tokens"] == 6

    details = jsn["usage"].get("completion_tokens_details")
    assert details is not None
    assert details["audio_tokens"] == 1
    assert details["rejected_prediction_tokens"] == 2
    assert details["accepted_prediction_tokens"] == 3
    assert details["reasoning_tokens"] == 4
