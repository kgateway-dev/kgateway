import copy
import json

from ext_proc.provider import Tokens, Anthropic, Gemini, OpenAI
from guardrails import api as webhook_api
from ext_proc.streamchunkdata import StreamChunkDataType
from typing import Dict, Any


def test_tokens_addition():
    tokens1 = Tokens(completion=5, prompt=10)
    tokens2 = Tokens(completion=3, prompt=7)
    result = tokens1 + tokens2
    assert result.completion == 8
    assert result.prompt == 17


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


def test_gemini_tokens():
    provider = Gemini()
    tokens = provider.tokens(gemini_resp())
    assert tokens.completion == 241
    assert tokens.prompt == 5


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


def test_openai_tokens():
    provider = OpenAI()
    tokens = provider.tokens(openai_resp())
    assert tokens.completion == 16
    assert tokens.prompt == 28


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
