import logging
from google.generativeai.types.answer_types import FinishReason as GeminiFinishReason
from google.generativeai.types import (
    FunctionDeclaration as GeminiFunctionDeclaration,
    Tool as GeminiTool,
)
from vertexai.generative_models import (
    FinishReason as VertexFinishReason,
)

from vertexai.generative_models import (
    Content,
    FunctionDeclaration,
    Part,
    Tool,
)

from client.client import LLMClient

logger = logging.getLogger(__name__)
logger.setLevel(logging.DEBUG)


class TestUserFunctionCalling(LLMClient):
    def test_openai_function_calling(self):
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

        resp = self.openai_chat_completion(
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
        assert len(resp.choices) > 0, "No choices found in the response"

        choice_message = resp.choices[0].message
        # check for tool_calls
        assert choice_message.tool_calls is not None, (
            "No tool calls found in the message"
        )
        assert choice_message.tool_calls[0].function.name == "get_weather", (
            "Incorrect function called"
        )

        # Validate the usage data is set for tool calls
        assert resp.usage is not None, "Usage is None"
        assert resp.usage.prompt_tokens > 0, "Prompt tokens are 0 or less"
        assert resp.usage.completion_tokens > 0, "Completion tokens are 0 or less"

        # Simulate the API response
        fake_api_response = """{
            "location": "Columbus, OH",
            "weather": "super nice",
        }"""
        # Second call with a fake function response
        second_resp = self.openai_chat_completion(
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
                            "id": choice_message.tool_calls[0].id,
                            "type": "function",
                            "function": {
                                "name": choice_message.tool_calls[0].function.name,
                                "arguments": choice_message.tool_calls[
                                    0
                                ].function.arguments,
                            },
                        }
                    ],
                },
                {
                    "role": "tool",
                    "content": fake_api_response,
                    "tool_call_id": choice_message.tool_calls[0].id,
                },
            ],
        )
        logger.debug(f"Second openai function call response:\n{second_resp}")
        assert second_resp is not None
        assert len(second_resp.choices) > 0
        assert second_resp.choices[0].message.content is not None
        assert "super nice" in second_resp.choices[0].message.content

    def test_azure_openai_completion(self):
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

        resp = self.azure_openai_client.chat.completions.create(
            model="gpt-4o-mini",
            tools=tools,
            messages=[
                {
                    "role": "user",
                    "content": "What is the weather in Columbus?",
                },
            ],
        )
        logger.debug(f"openai azure function call response:\n{resp}")
        assert resp is not None and len(resp.choices) > 0
        assert (
            resp.usage is not None
            and resp.usage.prompt_tokens > 0
            and resp.usage.completion_tokens > 0
        )

    def test_gemini_completion(self):
        # Specify a function declaration and parameters for an API request
        function_name = "get_current_weather"
        get_current_weather_func = GeminiFunctionDeclaration(
            name=function_name,
            description="Get the current weather in a given location",
            # Function parameters are specified in JSON schema format
            parameters={
                "type": "object",
                "properties": {
                    "location": {"type": "string", "description": "Location"}
                },
            },
        )

        # Define a tool that includes the above get_current_weather_func
        weather_tool = GeminiTool(
            function_declarations=[get_current_weather_func],
        )
        user_prompt_content = "What is the weather like in Columbus?"

        first_response = self.gemini_client.generate_content(
            user_prompt_content,
            tools=[weather_tool],
        )
        assert first_response is not None
        logger.debug(f"gemini completion response:\n{first_response}")
        assert len(first_response.candidates) == 1
        assert first_response.candidates[0].finish_reason == GeminiFinishReason.STOP
        assert first_response.usage_metadata.prompt_token_count > 0

        # Check the function name that the model responded with
        function_call = first_response.candidates[0].content.parts[0].function_call
        logger.debug(f"Suggested function call:\n {function_call}")
        assert function_call is not None
        assert function_name == function_call.name

        function_call_model_response = """
        {
          "functionCall": {
            "name": "get_current_weather",
            "args": {
              "location": "Columbus"
            }
          }
        }        
"""
        api_response = """
        {
            "functionResponse": {
                "name": "get_current_weather",
                "response": {
                "name": "get_current_weather",
                "content": { "location": "Columbus, OH", "weather": "super nice" } }
                }
            }
            }
        """
        second_response = self.gemini_client.generate_content(
            [
                user_prompt_content,  # User prompt
                function_call_model_response,  # Function call response
                api_response,  # API response
            ],
            tools=[weather_tool],
        )
        logger.debug(f"gemini completion response:\n{second_response}")
        assert second_response is not None
        assert "super nice" in second_response.candidates[0].content.parts[0].text

    def test_vertex_ai_completion(self):
        # Specify a function declaration and parameters for an API request
        function_name = "get_current_weather"
        get_current_weather_func = FunctionDeclaration(
            name=function_name,
            description="Get the current weather in a given location",
            # Function parameters are specified in JSON schema format
            parameters={
                "type": "object",
                "properties": {
                    "location": {"type": "string", "description": "Location"}
                },
            },
        )

        # Define a tool that includes the above get_current_weather_func
        weather_tool = Tool(
            function_declarations=[get_current_weather_func],
        )

        user_prompt_content = Content(
            role="user",
            parts=[
                Part.from_text("What is the weather like in Boston?"),
            ],
        )
        first_response = self.vertex_ai_client.generate_content(
            user_prompt_content,
            tools=[weather_tool],
        )
        assert first_response is not None
        logger.debug(f"Vertex AI completion response:\n{first_response}")
        assert len(first_response.candidates) == 1
        assert first_response.candidates[0].finish_reason == VertexFinishReason.STOP
        assert first_response.usage_metadata.prompt_token_count > 0

        function_call = first_response.candidates[0].function_calls[0]
        logger.debug(f"Suggested function calls:\n {function_call}")
        assert function_call is not None
        assert function_name == function_call.name

        api_response = """{ "location": "Columbus, OH", "weather": "super nice" } }"""

        # Return the API response to Gemini so it can generate a model response or request another function call
        part_content = Part.from_function_response(
            name=function_name,
            response={
                "content": api_response,  # Return the API response to Gemini
            },
        )

        second_response = self.vertex_ai_client.generate_content(
            contents=[
                user_prompt_content,  # User prompt
                first_response.candidates[0].content,  # Function call response
                Content(parts=[part_content]),
            ],
            tools=[weather_tool],
        )
        logger.debug(f"second response: {second_response}")
        assert second_response is not None
        assert len(second_response.candidates) == 1
        assert second_response.candidates[0].finish_reason == VertexFinishReason.STOP
        assert "super nice" in second_response.candidates[0].content.parts[0].text
