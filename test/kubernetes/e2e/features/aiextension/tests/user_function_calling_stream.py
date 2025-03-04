import logging
from client.client import LLMClient
from util.gemini import helpers as gemini_helpers

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestUserFunctionCalling(LLMClient):
    def test_openai_function_calling_streaming(self):
        tools = [
            {
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "Get the current weather in a given location",
                    "parameters": {
                        "type": "object",
                        "properties": {"location": {"type": "string"}},
                    },
                },
            }
        ]

        resp = self.openai_chat_completion_stream(
            model="gpt-4o-mini",
            tools=tools,
            messages=[
                {
                    "role": "user",
                    "content": "What is the weather in Columbus?",
                },
            ],
        )
        logger.debug(f"openai function call response:\n{resp}")

        # Ensure the response is valid
        assert resp is not None, "Response is None"
        function_arguments = ""
        function_name = ""
        tool_call_id = ""

        last_chunk = None
        for part in resp:
            logger.debug(f"openai completion stream chunk:\n{part}")
            assert part is not None
            if len(part.choices) > 0:
                last_chunk = part
            delta = part.choices[0].delta

            # Process assistant content
            if hasattr(delta, "content"):
                if delta.tool_calls:
                    tool_call = delta.tool_calls[0]

                    if tool_call.function and tool_call.function.name:
                        function_name = tool_call.function.name

                    if tool_call.function and tool_call.function.arguments:
                        function_arguments += tool_call.function.arguments

                    if tool_call.id:
                        tool_call_id = tool_call.id

        assert (
            last_chunk is not None
            and len(last_chunk.choices) > 0
            and last_chunk.choices[0].finish_reason == "tool_calls"
        )
        logger.debug(
            f"Function call '{function_name}' is complete. Args: {function_arguments}. Tool call ID: {tool_call_id}\n"
        )

        # Simulate the API response
        fake_api_response = """{
            "location": "Columbus, OH",
            "weather": "super nice",
        }"""
        # Second call with a fake function response
        second_resp = self.openai_chat_completion_stream(
            model="gpt-4o-mini",
            tools=tools,
            messages=[
                {
                    "role": "user",
                    "content": "What is the weather in Columbus?",
                },
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [
                        {
                            "id": tool_call_id,
                            "type": "function",
                            "function": {
                                "name": function_name,
                                "arguments": function_arguments,
                            },
                        }
                    ],
                },
                {
                    "role": "tool",
                    "content": fake_api_response,
                    "tool_call_id": tool_call_id,
                },
            ],
        )
        logger.debug(f"Second openai function call response:\n{second_resp}")
        assert second_resp is not None
        last_chunk = None
        chunks_str = ""
        for chunk in second_resp:
            logger.debug(f"Second openai completion stream chunk:\n{chunk}")
            assert chunk is not None
            if len(chunk.choices) > 0:
                last_chunk = chunk
            chunks_str += str(chunk.choices[0].delta.content)
        assert chunks_str != ""
        assert "super nice" in chunks_str

    def test_gemini_function_calling_streaming(self):
        tools = [
            {
                "functionDeclarations": [
                    {
                        "name": "get_weather",
                        "description": "Get the current weather in a given location",
                        "parameters": {
                            "type": "object",
                            "properties": {
                                "location": {
                                    "type": "string",
                                    "description": "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616",
                                }
                            },
                            "required": ["location"],
                        },
                    }
                ]
            }
        ]
        resp = gemini_helpers.make_stream_request(
            provider="gemini", instruction="What is the weather in Boston?", tools=tools
        )
        assert resp is not None
        assert resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        _, complete_response, _, _, function_call = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(resp, "", 0)
        )
        assert function_call is not None
        logger.debug(f"Function call: {function_call}")
        assert function_call["name"] == "get_weather"

        addition_contents = [
            {
                "role": "assistant",
                "parts": [
                    {
                        "functionCall": {
                            "name": "get_weather",
                            "args": {"location": "Boston"},
                        }
                    }
                ],
            },
            {
                "role": "user",
                "parts": [
                    {
                        "functionResponse": {
                            "name": "get_weather",
                            "response": {
                                "name": "get_weather",
                                "content": {"weather": "very sunny"},
                            },
                        }
                    }
                ],
            },
        ]
        second_resp = gemini_helpers.make_stream_request(
            provider="gemini",
            instruction="What is the weather in Boston?",
            tools=tools,
            addition_contents=addition_contents,
        )
        assert second_resp is not None
        assert second_resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        _, complete_response, _, _, _ = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(second_resp, "", 0)
        )
        logger.debug(f"Gemini complete response:\n{second_resp}")
        assert "very sunny" in complete_response

    def test_vertex_ai_function_calling_streaming(self):
        tools = [
            {
                "functionDeclarations": [
                    {
                        "name": "get_weather",
                        "description": "Get the current weather in a given location",
                        "parameters": {
                            "type": "object",
                            "properties": {
                                "location": {
                                    "type": "string",
                                    "description": "The city and state, e.g. San Francisco, CA or a zip code e.g. 95616",
                                }
                            },
                            "required": ["location"],
                        },
                    }
                ]
            }
        ]
        resp = gemini_helpers.make_stream_request(
            provider="vertex_ai",
            instruction="What is the weather in Boston?",
            tools=tools,
        )
        assert resp is not None
        assert resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        _, complete_response, _, _, function_call = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(resp, "", 0)
        )
        logger.debug(f"Vertex AI complete response:\n{complete_response}")
        assert function_call is not None
        logger.debug(f"Function call: {function_call}")
        assert function_call["name"] == "get_weather"

        addition_contents = [
            {
                "role": "assistant",
                "parts": [
                    {
                        "functionCall": {
                            "name": "get_weather",
                            "args": {"location": "Boston"},
                        }
                    }
                ],
            },
            {
                "role": "user",
                "parts": [
                    {
                        "functionResponse": {
                            "name": "get_weather",
                            "response": {
                                "name": "get_weather",
                                "content": {"weather": "very sunny"},
                            },
                        }
                    }
                ],
            },
        ]
        second_resp = gemini_helpers.make_stream_request(
            provider="vertex_ai",
            instruction="What is the weather in Boston?",
            tools=tools,
            addition_contents=addition_contents,
        )
        assert second_resp is not None
        assert second_resp.status_code == 200, "Failed to get a successful response"
        assert "text/event-stream" in resp.headers.get("Content-Type", ""), (
            "Unexpected content type"
        )
        _, complete_response, _, _, _ = (
            gemini_helpers.count_pattern_and_extract_data_in_chunks(second_resp, "", 0)
        )
        logger.debug(f"Vertex AI complete response:\n{second_resp}")
        assert "very sunny" in complete_response
