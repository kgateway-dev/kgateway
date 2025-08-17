import logging
import tiktoken
import traceback

from typing import Any, Callable, Dict, Final, List, Optional

from api.envoy.config.core.v3 import base_pb2 as base_pb2
from dataclasses import dataclass
from guardrails import api as webhook_api
from abc import ABC, abstractmethod
from util.http import get_content_type
from opentelemetry.util.types import Attributes
from opentelemetry.semconv._incubating.attributes import gen_ai_attributes

from ext_proc.streamchunkdata import StreamChunkData, StreamChunkDataType

logger = logging.getLogger().getChild("kgateway-ai-ext.provider")

# LLM providers from header value
OPENAI_LLM_STR: Final[str] = "openai"
ANTHROPIC_LLM_STR: Final[str] = "anthropic"
GEMINI_LLM_STR: Final[str] = "gemini"
VERTEX_AI_LLM_STR: Final[str] = "vertex-ai"


@dataclass
class TokensDetails:
    """
    Detail breakdowns of the prompt or completion tokens reported
    OpenAI reference: https://platform.openai.com/docs/api-reference/chat/object#chat/object-usage
    Gemini reference: https://ai.google.dev/api/generate-content#UsageMetadata
    """

    cached: int = 0  # prompt (Gemini, OpenAI)
    tool_used: int = 0  # prompt (Gemini)
    accepted_prediction: int = 0  # completion (OpenAI)
    rejected_prediction: int = 0  # completion (OpenAI)
    reasoning: int = 0  # completion (OpenAI)
    text: int = 0  # prompt or completion (Gemini)
    audio: int = 0  # prompt or completion (Gemini, OpenAI)
    document: int = 0  # prompt or completion (Gemini)
    image: int = 0  # prompt or completion (Gemini)
    video: int = 0  # prompt or completion (Gemini)

    def __add__(self, other):
        if other is None:
            return self

        return TokensDetails(
            cached=self.cached + other.cached,
            tool_used=self.tool_used + other.tool_used,
            accepted_prediction=self.accepted_prediction + other.accepted_prediction,
            rejected_prediction=self.rejected_prediction + other.rejected_prediction,
            reasoning=self.reasoning + other.reasoning,
            text=self.text + other.text,
            audio=self.audio + other.audio,
            document=self.document + other.document,
            image=self.image + other.image,
            video=self.video + other.video,
        )


@dataclass
class Tokens:
    """
    Tokens is a dataclass that represents the tokens used in a request and response.
    """

    completion: int = 0
    prompt: int = 0
    prompt_details: Optional[TokensDetails] = None
    completion_details: Optional[TokensDetails] = None

    def __add_optional_details(
        self, a: Optional[TokensDetails], b: Optional[TokensDetails]
    ) -> TokensDetails | None:
        if a is None:
            return b

        if b is None:
            return a

        return a + b

    def __add__(self, other):
        return Tokens(
            completion=self.completion + other.completion,
            prompt=self.prompt + other.prompt,
            prompt_details=self.__add_optional_details(
                self.prompt_details, other.prompt_details
            ),
            completion_details=self.__add_optional_details(
                self.completion_details, other.completion_details
            ),
        )

    def total_tokens(self) -> int:
        return self.completion + self.prompt


class Provider(ABC):
    """
    Provider is an abstract class that defines the interface for a provider.

    The provider is responsible for interfacing with the different request and
    response formats of the different AI providers.
    """

    @abstractmethod
    def get_attributes_for_response_body(self, jsn: dict) -> Attributes:
        pass

    @abstractmethod
    def tokens(self, jsn: dict) -> Tokens:
        """
        tokens should return the Tokens object from the JSON response of the provider.
        """
        pass

    @abstractmethod
    def create_usage_json(self, tokens: Tokens) -> Dict[str, Any]:
        """
        create the usage json object base on the values in tokens
        """
        pass

    def has_tools_defined(self, body: dict) -> bool:
        """
        has_tools_defined should return a boolean indicating if the request to the provider
        contains any function call definition.
        """
        # check if the body has "tools" field defined
        return "tools" in body

    @abstractmethod
    def has_function_call_finish_reason(self, body: dict) -> bool:
        """
        has_function_call_finish_reason should return a boolean indicating if the response
        from the provider has a function call finish reason, which indicates the user needs
        to make the call, then send the request with the result.
        """
        pass

    @abstractmethod
    def update_stream_resp_usage_token(self, json_data: Dict[str, Any], tokens: Tokens):
        """
        update the usage data in json_data with the usage number in tokens
        """
        pass

    @abstractmethod
    def get_num_tokens_from_body(self, body: dict) -> int:
        """
        get_num_tokens_from_body should return the number of tokens from the body of the request.
        """
        pass

    @abstractmethod
    def iterate_str_resp_messages(self, body: dict, cb: Callable[[str, str], str]):
        """
        The cb function is called with the role and content of each message in the response body.
        The return value of the cb function is used to replace the content of the message.
        """
        pass

    @abstractmethod
    def iterate_str_req_messages(self, body: dict, cb: Callable[[str, str], str]):
        """
        The cb function is called with the role and content of each message in the request body.
        The return value of the cb function is used to replace the content of the message.
        """
        pass

    @abstractmethod
    def get_model_req(self, body_jsn: dict, headers_jsn: dict) -> str:
        """
        get_model_req should return the model requested from the client.
        """
        pass

    @abstractmethod
    def get_model_resp(self, body_jsn: dict) -> str:
        """
        get_model_resp should return the model used by the provider.
        """
        pass

    @abstractmethod
    def is_streaming_req(self, body_jsn: dict, headers_jsn: dict) -> bool:
        """
        is_streaming_req should return a boolean indicating if the request is a streaming request.
        """
        pass

    @abstractmethod
    def is_streaming_response(
        self,
        is_streaming_request: bool,
        response_headers: base_pb2.HeaderMap,
        content_type: str | None = None,
    ) -> bool:
        """
        is_streaming_response should return a boolean indicating if the response is a streaming response.
        """
        pass

    @abstractmethod
    def extract_contents_from_resp_chunk(
        self, json_data: Dict[str, Any] | None
    ) -> List[bytes] | None:
        """
        extract the content texts from the response chunk in streaming response.
        """
        pass

    @abstractmethod
    def has_choice_index(
        self, json_data: Dict[str, Any] | None, choice_index: int
    ) -> bool:
        """
        check if the choices inside the json_data has the specified choice_index
        """
        pass

    @abstractmethod
    def update_stream_resp_contents(
        self, json_data: Dict[str, Any] | None, choice_index: int, content: bytes
    ):
        """
        update the content texts in the stream response json_data.
        """
        pass

    @abstractmethod
    def get_sse_delimiter(self) -> bytes:
        """
        The delimiter that separate SSE message block is normally "\n\n" but Gemini uses "\r\n\r\n" instead
        """
        pass

    @abstractmethod
    def get_stream_resp_chunk_type(
        self, json_data: Dict[str, Any]
    ) -> StreamChunkDataType:
        """
        This is the function to get the chunk type which is currently used when collapsing chunks after
        guardrail modification but will also be used to control if we pause buffering in the future when
        we encounter binary data in between texts in a single response.
        """
        pass

    @abstractmethod
    def all_req_content(self, body: dict) -> str:
        """
        Get all content from the request body. Specifically this will
        navigate through the messages and return all text content combined.
        """
        pass

    @abstractmethod
    def construct_request_webhook_request_body(
        self, body: dict
    ) -> webhook_api.PromptMessages:
        """
        Extract the "role" and "content" from the request body and construct the
        unified webhook request body according to the class "PromptMessages" defined
        in guardrails/api.py
        """
        pass

    @abstractmethod
    def update_request_body_from_webhook(
        self, original_body: dict, webhook_modified_messages: webhook_api.PromptMessages
    ):
        """
        Update the request body content with the modified version from webhook response.
        """
        pass

    @abstractmethod
    def construct_response_webhook_request_body(
        self, body: dict
    ) -> webhook_api.ResponseChoices:
        """
        Extract the "content" from the response body and construct the
        unified webhook request body according to the class "ResponseChoices" defined
        in guardrails/api.py.
        This function is used for non-stream response only
        """
        pass

    @abstractmethod
    def update_response_body_from_webhook(
        self,
        original_body: dict,
        webhook_modified_messages: webhook_api.ResponseChoices,
    ):
        """
        Update the response body content with the modified version from webhook response.
        This function is used for non-stream response only
        """
        pass

    @abstractmethod
    def is_streaming_response_completed(
        self,
        chunk: StreamChunkData,
    ) -> bool:
        """
        Handle provider-specific processing for checking if we have reach the end of the streaming response
        """
        pass


def content_from_dict(content: dict) -> str:
    if content.get("type", "") == "text":
        return content["text"]
    raise ValueError(f"Unsupported content type: {content}")


class OpenAI(Provider):
    def get_attributes_for_response_body(self, body: dict) -> Attributes:
        finish_reason = ""

        if isinstance(body.get("choices"), list) and len(body["choices"]) > 0:
            first_choice = body["choices"][0]
            if isinstance(first_choice, dict):
                finish_reason = first_choice.get("finish_reason", "")

        return {
            gen_ai_attributes.GEN_AI_RESPONSE_ID: body.get("id", ""),
            gen_ai_attributes.GEN_AI_RESPONSE_MODEL: self.get_model_resp(body),
            gen_ai_attributes.GEN_AI_RESPONSE_FINISH_REASONS: finish_reason,
            gen_ai_attributes.GEN_AI_USAGE_INPUT_TOKENS: self.tokens(body).prompt,
            gen_ai_attributes.GEN_AI_USAGE_OUTPUT_TOKENS: self.tokens(body).completion,
        }

    def tokens(self, jsn: dict) -> Tokens:
        if "usage" not in jsn:
            # streaming chunk by default does not contain usage
            return Tokens()

        usage = jsn.get("usage")
        if usage is None:
            # streaming chunk would have null usage (if enabled) in every chunk
            # and only the total will be in the last chunk
            return Tokens()

        prompt_details = None
        details = usage.get("prompt_tokens_details")
        if details is not None:
            prompt_details = TokensDetails(
                audio=details.get("audio_tokens", 0),
                cached=details.get("cached_tokens", 0),
            )

        completion_details = None
        details = usage.get("completion_tokens_details")
        if details is not None:
            completion_details = TokensDetails(
                audio=details.get("audio_tokens", 0),
                accepted_prediction=details.get("accepted_prediction_tokens", 0),
                rejected_prediction=details.get("rejected_prediction_tokens", 0),
                reasoning=details.get("reasoning_tokens", 0),
            )

        return Tokens(
            completion=int(usage.get("completion_tokens", 0)),
            prompt=int(usage.get("prompt_tokens", 0)),
            prompt_details=prompt_details,
            completion_details=completion_details,
        )

    def create_usage_json(self, tokens: Tokens) -> Dict[str, Any]:
        # As of 2/27/2025, the OpenAI usage object looks like this:
        # {
        #   "completion_tokens": 68,
        #   "completion_tokens_details": {
        #     "accepted_prediction_tokens": 0,
        #     "audio_tokens": 0,
        #     "reasoning_tokens": 0,
        #     "rejected_prediction_tokens": 0
        #   },
        #   "prompt_tokens": 84,
        #   "prompt_tokens_details": {
        #     "audio_tokens": 0,
        #     "cached_tokens": 0
        #   },
        #   "total_tokens": 152
        # }
        # currently, we only need to create the usage json when collapsing
        # chunks that has usage but OpenAI only send usage one time at the end
        # of the stream, so most likely we don't need this but just in case
        # they change the behavior in the future, we will be covered.
        usage = {}
        usage["completion_tokens"] = tokens.completion
        usage["prompt_tokens"] = tokens.prompt
        usage["total_tokens"] = tokens.total_tokens()
        if tokens.prompt_details is not None:
            usage["prompt_tokens_details"] = {
                "audio_tokens": tokens.prompt_details.audio,
                "cached_tokens": tokens.prompt_details.cached,
            }

        if tokens.completion_details is not None:
            usage["completion_tokens_details"] = {
                "audio_tokens": tokens.completion_details.audio,
                "accepted_prediction_tokens": tokens.completion_details.accepted_prediction,
                "reasoning_tokens": tokens.completion_details.reasoning,
                "rejected_prediction_tokens": tokens.completion_details.rejected_prediction,
            }

        return usage

    def has_function_call_finish_reason(self, body: dict) -> bool:
        # check if any of the choices have the "finish_reason" field defined as function_call
        if "choices" in body:
            for choice in body["choices"]:
                if (
                    "finish_reason" in choice
                    and choice["finish_reason"] == "tool_calls"
                ):
                    return True
                # for streaming responses, the finish_reason is in the last chunk, but every chunk will have a `tool_calls` delta.
                if "delta" in choice and "tool_calls" in choice["delta"]:
                    return True
        return False

    def update_stream_resp_usage_token(self, json_data: Dict[str, Any], tokens: Tokens):
        # for OpenAI streaming response, while each chunk has "usage" field but they are always null
        # the total usage is in the last chunk
        json_data["usage"] = self.create_usage_json(tokens)

    def iterate_str_resp_messages(self, body: dict, cb: Callable[[str, str], str]):
        for idx, choice in enumerate(body.get("choices", [])):
            if isinstance(choice["message"]["content"], str):
                body["choices"][idx]["message"]["content"] = cb(
                    choice["message"]["role"], choice["message"]["content"]
                )
                return choice["message"]["content"]
            elif isinstance(choice["message"]["content"], dict):
                body["choices"][idx]["message"]["content"]["text"] = cb(
                    choice["message"]["role"],
                    content_from_dict(choice["message"]["content"]),
                )

    def iterate_str_req_messages(self, body: dict, cb: Callable[[str, str], str]):
        for idx, message in enumerate(body.get("messages", [])):
            if isinstance(message["content"], str):
                body["messages"][idx]["content"] = cb(
                    message["role"], message["content"]
                )
            elif isinstance(message["content"], dict):
                body["messages"][idx]["content"]["text"] = cb(
                    message["role"], content_from_dict(message["content"])
                )

    def get_num_tokens_from_body(self, body: dict) -> int:
        tokens: int = 0
        if "messages" in body:
            messages = body["messages"]
            tokens = num_tokens_from_messages(messages)
        elif "input" in body:
            tokens = num_tokens_from_messages(body["input"])
        elif "prompt" in body:
            # image generation
            tokens = num_tokens_from_messages(body["prompt"])
        return tokens

    def get_model_req(self, body_jsn: dict, headers_jsn: dict) -> str:
        return body_jsn.get("model", "")

    def get_model_resp(self, body_jsn: dict) -> str:
        return body_jsn.get("model", "")

    def is_streaming_req(self, body_jsn: dict, headers_jsn: dict) -> bool:
        return body_jsn.get("stream", False)

    def is_streaming_response(
        self,
        is_streaming_request: bool,
        response_headers: base_pb2.HeaderMap,
        content_type: str | None = None,
    ) -> bool:
        if content_type is None:
            content_type = get_content_type(response_headers)

        if content_type == "text/event-stream":
            return True

        return False

    def all_req_content(self, body: dict) -> str:
        s = ""
        messages = body.get("messages", [])
        for i, message in enumerate(messages):
            s += f"role: {message['role']}:\n"
            if isinstance(message["content"], str):
                s += message["content"]
            elif isinstance(message["content"], dict):
                s += content_from_dict(message["content"])
            elif isinstance(message["content"], list):
                for content in message["content"]:
                    s += content_from_dict(content)
            if i != len(messages) - 1:
                s += "\n"
        return s

    def construct_request_webhook_request_body(
        self, body: dict
    ) -> webhook_api.PromptMessages:
        prompt_messages = webhook_api.PromptMessages()

        messages = body.get("messages", [])
        for message in messages:
            prompt_messages.messages.append(
                webhook_api.Message(
                    role=message.get("role", ""), content=message.get("content", "")
                )
            )
        return prompt_messages

    def update_request_body_from_webhook(
        self, original_body: dict, webhook_modified_messages: webhook_api.PromptMessages
    ):
        messages = original_body.get("messages", [])
        if len(messages) != len(webhook_modified_messages.messages):
            logger.error(
                "webhook modified messages do not match the original messages array size!"
            )
            return

        for i, modified_message in enumerate(webhook_modified_messages.messages):
            if "role" in messages[i] and messages[i]["role"] != modified_message.role:
                logger.warning(
                    "webhook modified messages attempts to modify the role from %s to %s. ignoring.",
                    messages[i]["role"],
                    modified_message.role,
                )

            messages[i]["content"] = modified_message.content

    def construct_response_webhook_request_body(
        self, body: dict
    ) -> webhook_api.ResponseChoices:
        response_choices = webhook_api.ResponseChoices()
        for orig_choice in body.get("choices", []):
            if "message" in orig_choice and "content" in orig_choice["message"]:
                response_choices.choices.append(
                    webhook_api.ResponseChoice(
                        message=webhook_api.Message(
                            role=orig_choice["message"].get("role", ""),
                            content=orig_choice["message"]["content"],
                        )
                    )
                )
            else:
                # Still need to append a choice so the len of the array will match the original
                # if webhook modifies the content
                response_choices.choices.append(
                    webhook_api.ResponseChoice(
                        message=webhook_api.Message(role="", content="")
                    )
                )

        return response_choices

    def update_response_body_from_webhook(
        self,
        original_body: dict,
        webhook_modified_messages: webhook_api.ResponseChoices,
    ):
        choices = original_body.get("choices", [])
        if len(choices) != len(webhook_modified_messages.choices):
            logger.error(
                "webhook modified messages do not match the original choices array size!"
            )
            return

        for i, modified_content in enumerate(webhook_modified_messages.choices):
            if "message" not in choices[i]:
                continue

            orig_role = choices[i]["message"].get("role", "")
            if orig_role != modified_content.message.role:
                logger.warning(
                    "webhook modified messages attempts to modify the role from %s to %s. ignoring.",
                    orig_role,
                    modified_content.message.role,
                )

            if "content" in choices[i]["message"]:
                choices[i]["message"]["content"] = modified_content.message.content

    def extract_contents_from_resp_chunk(self, json_data) -> List[bytes] | None:
        if json_data is None:
            return None

        contents: List[bytes] = []
        for choice in json_data["choices"]:
            # For OpenAI streaming response with multi-choices, the choices array always has a single choice
            # the index is the indicator of which choice the content belongs to
            if "content" in choice["delta"]:
                index = choice.get("index", 0)
                # The generic interface of this function can still work with providers that send multiple
                # choices in a single array (currently, no provider does that. In fact, only OpenAI supports multi-choice at all)
                # So, need to make sure we pad out the contents list to put the content in the correct slot base on the index
                contents.extend([b""] * (index + 1 - len(contents)))
                contents[index] = choice["delta"]["content"].encode("utf-8")
                logger.debug(
                    f"content from json: {choice['delta']['content']} [{len(choice['delta']['content'])}] content after encoding: {contents[index]} [{len(contents[index])}]"
                )

        if len(contents) == 0:
            return None

        logger.debug(f"contents: {contents}")
        return contents

    def has_choice_index(
        self, json_data: Dict[str, Any] | None, choice_index: int
    ) -> bool:
        if json_data is None:
            logger.warning(
                f"has_choice_index() called by no json_data. choice_index: {choice_index}"
            )
            return False

        if len(json_data.get("choices", [])) == 0:
            logger.error(
                "has_choice_index() called by no choices in json_data. choice_index: {}",
                choice_index,
            )
            return False

        choice = json_data["choices"][0]
        if len(json_data["choices"]) > 1:
            for c in json_data["choices"]:
                if c.get("index", 0) == choice_index:
                    choice = c

        if choice.get("index", 0) == choice_index:
            return True

        return False

    def update_stream_resp_contents(self, json_data, choice_index: int, content: bytes):
        if json_data is None or not self.has_choice_index(json_data, choice_index):
            logger.warning(
                f"update_stream_resp_contents() called but does not have choice_index: {choice_index} content: {content}"
            )
            return None

        # The 0 index looks odd here but it's because OpenAI only put a single choice in the choices array
        # and uses the index field to indicate the choice_index instead of the array's index.
        choice = json_data["choices"][0]
        if len(json_data["choices"]) > 1:
            # This is just in case that OpenAI starts doing what that says in their API doc,
            # So, we walk the choices and check the index field to see which entry matches the choice_index
            # and update our selection.
            for c in json_data["choices"]:
                if c["index"] == choice_index:
                    choice = c

        if "delta" not in choice or "content" not in choice["delta"]:
            return None

        logger.debug(
            f"update_stream_resp_contents: before: '{choice['delta']['content']}'"
        )
        choice["delta"]["content"] = content.decode("utf-8")
        logger.debug(
            f"update_stream_resp_contents: after : '{choice['delta']['content']}'"
        )
        logger.debug(f"update_stream_resp_contents: json_data after: {json_data}")

    def is_streaming_response_completed(
        self,
        chunk: StreamChunkData,
    ) -> bool:
        if chunk.type == StreamChunkDataType.DONE:
            return True

        return False

    def get_sse_delimiter(self) -> bytes:
        return b"\n\n"

    def get_stream_resp_chunk_type(
        self, json_data: Dict[str, Any]
    ) -> StreamChunkDataType:
        try:
            has_content = False
            has_audio = False
            if len(json_data["choices"]) > 0:
                choice0 = json_data["choices"][0]
                if "delta" in choice0:
                    if "content" in choice0["delta"]:
                        has_content = True
                    if "audio" in choice0["delta"]:
                        has_audio = True

                if "finish_reason" in choice0 and choice0["finish_reason"] is not None:
                    if has_content:
                        return StreamChunkDataType.FINISH
                    else:
                        return StreamChunkDataType.FINISH_NO_CONTENT
                else:
                    if has_audio:
                        return StreamChunkDataType.NORMAL_BINARY
                    else:
                        return StreamChunkDataType.NORMAL_TEXT
            elif json_data.get("usage", None) is not None:
                return StreamChunkDataType.LAST_USAGE
            else:
                return StreamChunkDataType.UNKNOWN

        except Exception as e:
            logger.warning(f"invalid chunk: exception: {e} json_data: {json_data}")
            logger.debug(traceback.format_exc())
            return StreamChunkDataType.INVALID


class Anthropic(OpenAI):
    def get_attributes_for_response_body(self, jsn: dict) -> Attributes:
        # TODO(zhengke) implement me
        return {}

    def tokens(self, jsn: dict) -> Tokens:
        if "usage" not in jsn:
            return Tokens()

        return Tokens(
            completion=jsn.get("usage", {}).get("output_tokens", 0),
            prompt=jsn.get("usage", {}).get("input_tokens", 0),
        )

    def create_usage_json(self, tokens: Tokens) -> Dict[str, Any]:
        return {"input_tokens": tokens.prompt, "output_tokens": tokens.completion}

    def get_model_resp(self, body_jsn: dict) -> str:
        if body_jsn.get("type", "") == "message_start" and "message" in body_jsn:
            return body_jsn["message"].get("model", "")

        return body_jsn.get("model", "")

    def has_function_call_finish_reason(self, body: dict) -> bool:
        # check if response content has a tool_use field in the response.
        if "content" in body:
            for content in body["content"]:
                if "tool_use" in content:
                    return True
        return False

    def update_stream_resp_usage_token(self, json_data: Dict[str, Any], tokens: Tokens):
        # TODO (andy): Have not seen per chunk usage data from their example, will need to test this out
        pass

    def iterate_str_resp_messages(self, body: dict, cb: Callable[[str, str], str]):
        match body.get("type"):
            case "message":
                for idx, message in enumerate(body.get("content", [])):
                    match message.get("type"):
                        case "text":
                            body["content"][idx]["text"] = cb(
                                body.get("role", ""), message.get("text")
                            )
                            pass
                        case _:
                            pass
            case _:
                pass

    def get_stream_resp_chunk_type(
        self, json_data: Dict[str, Any]
    ) -> StreamChunkDataType:
        # TODO(andy): to be implemented
        return StreamChunkDataType.NORMAL_TEXT

    def construct_response_webhook_request_body(
        self, body: dict
    ) -> webhook_api.ResponseChoices:
        response_choices = webhook_api.ResponseChoices()
        for orig_content in body.get("content", []):
            if "text" in orig_content:
                response_choices.choices.append(
                    webhook_api.ResponseChoice(
                        message=webhook_api.Message(
                            role=body.get("role", ""),
                            content=orig_content["text"],
                        )
                    )
                )
            else:
                # Still need to append a choice so the len of the array will match the original
                # if webhook modifies the content
                response_choices.choices.append(
                    webhook_api.ResponseChoice(
                        message=webhook_api.Message(role="", content="")
                    )
                )

        return response_choices

    def update_response_body_from_webhook(
        self,
        original_body: dict,
        webhook_modified_messages: webhook_api.ResponseChoices,
    ):
        contents = original_body.get("content", [])
        if len(contents) != len(webhook_modified_messages.choices):
            logger.error(
                "webhook modified messages do not match the original contents array size!"
            )
            return

        for i, modified_content in enumerate(webhook_modified_messages.choices):
            if "text" in contents[i]:
                contents[i]["text"] = modified_content.message.content

        return None

    def extract_contents_from_resp_chunk(
        self, json_data: Dict[str, Any] | None
    ) -> List[bytes] | None:
        if json_data is None:
            return None

        if json_data["type"] == "content_block_delta":
            if "text" not in json_data["delta"]:
                return None

            contents: List[bytes] = []
            contents.append(json_data["delta"]["text"].encode("utf-8"))

            return contents

        if json_data["type"] == "content_block_start":
            if (
                "content_block" not in json_data
                or "text" not in json_data["content_block"]
            ):
                return None

            contents: List[bytes] = []
            contents.append(json_data["content_block"]["text"].encode("utf-8"))

            return contents

        return None

    def has_choice_index(
        self, json_data: Dict[str, Any] | None, choice_index: int
    ) -> bool:
        # Anthropic does not support choices in response, so return false if choice_index is not 0
        return choice_index == 0

    def update_stream_resp_contents(self, json_data, choice_index: int, content: bytes):
        # Anthropic does not support choices in response, so choice_index is not used here
        if json_data is None:
            logger.warning(
                f"update_stream_resp_contents() called by no json_data. choice_index: {choice_index} content: {content}"
            )
            return None

        if json_data["type"] == "content_block_delta":
            if "text" not in json_data["delta"]:
                return None

            json_data["delta"]["text"] = content.decode("utf-8")

        if json_data["type"] == "content_block_start":
            if (
                "content_block" not in json_data
                or "text" not in json_data["content_block"]
            ):
                return None

            json_data["content_block"]["text"] = content.decode("utf-8")

        return None

    def is_streaming_response_completed(
        self,
        chunk: StreamChunkData,
    ) -> bool:
        if (
            chunk.json_data is not None
            and chunk.json_data.get("type", "") == "message_stop"
        ):
            return True

        return False


class Gemini(Provider):
    def get_attributes_for_response_body(self, jsn: dict) -> Attributes:
        # TODO(zhengke) implement me
        return {}

    def get_tokens_details_from_json(self, details_json: List[Any]) -> TokensDetails:
        tokens_details = TokensDetails()
        for detail in details_json:
            match detail.get("modality", ""):
                case "TEXT":
                    tokens_details.text = detail.get("tokenCount", 0)
                case "AUDIO":
                    tokens_details.audio = detail.get("tokenCount", 0)
                case "VIDEO":
                    tokens_details.video = detail.get("tokenCount", 0)
                case "DOCUMENT":
                    tokens_details.document = detail.get("tokenCount", 0)
                case "IMAGE":
                    tokens_details.image = detail.get("tokenCount", 0)

        return tokens_details

    def tokens(self, jsn: dict) -> Tokens:
        if "usageMetadata" not in jsn:
            # During testing, I observed, occasionally, a chunk will be missing usageMetadata
            return Tokens()

        usage = jsn.get("usageMetadata", {})

        prompt_details = None
        details = usage.get("promptTokensDetails")
        if details is not None and len(details) > 0:
            prompt_details = self.get_tokens_details_from_json(details)

        completion_details = None
        details = usage.get("candidatesTokensDetails")
        if details is not None and len(details) > 0:
            completion_details = self.get_tokens_details_from_json(details)

        return Tokens(
            completion=int(usage.get("candidatesTokenCount", 0)),
            prompt=int(usage.get("promptTokenCount", 0)),
            prompt_details=prompt_details,
            completion_details=completion_details,
        )

    def create_modality_json(self, modality: str, token_count: int) -> Dict[str, Any]:
        return {"modality": modality, "tokenCount": token_count}

    def create_tokens_details_json(self, tokens_details: TokensDetails) -> List[Any]:
        details = []
        if tokens_details.text > 0:
            details.append(self.create_modality_json("TEXT", tokens_details.text))
        if tokens_details.image > 0:
            details.append(self.create_modality_json("IMAGE", tokens_details.image))
        if tokens_details.audio > 0:
            details.append(self.create_modality_json("AUDIO", tokens_details.audio))
        if tokens_details.video > 0:
            details.append(self.create_modality_json("VIDEO", tokens_details.video))
        if tokens_details.document > 0:
            details.append(
                self.create_modality_json("DOCUMENT", tokens_details.document)
            )

        return details

    def create_usage_json(self, tokens: Tokens) -> Dict[str, Any]:
        # as of 2/27/2025, the gemini usage object looks like this:
        # "usageMetadata": {
        #   "promptTokenCount": 20,
        #   "candidatesTokenCount": 283,
        #   "totalTokenCount": 303,
        #   "promptTokensDetails": [
        #     {
        #       "modality": "TEXT",
        #       "tokenCount": 20
        #     }
        #   ],
        #   "candidatesTokensDetails": [
        #     {
        #       "modality": "TEXT",
        #       "tokenCount": 283
        #     }
        #   ]
        # }
        usageMetadata = {}
        usageMetadata["promptTokenCount"] = tokens.prompt
        usageMetadata["candidatesTokenCount"] = tokens.completion
        usageMetadata["totalTokenCount"] = tokens.total_tokens()
        if tokens.prompt_details is not None:
            tokens_details = self.create_tokens_details_json(tokens.prompt_details)
            if len(tokens_details) > 0:
                usageMetadata["promptTokensDetails"] = tokens_details

        if tokens.completion_details is not None:
            tokens_details = self.create_tokens_details_json(tokens.completion_details)
            if len(tokens_details) > 0:
                usageMetadata["candidatesTokensDetails"] = tokens_details

        return usageMetadata

    def has_function_call_finish_reason(self, body: dict) -> bool:
        # check if any of the candidates in the response have a functionCall defined
        if "candidates" in body:
            for candidate in body["candidates"]:
                if "content" in candidate:
                    for part in candidate["content"].get("parts", []):
                        if "functionCall" in part:
                            return True
        return False

    def update_stream_resp_usage_token(self, json_data: Dict[str, Any], tokens: Tokens):
        json_data["usageMetadata"] = self.create_usage_json(tokens)

    def get_sse_delimiter(self) -> bytes:
        return b"\r\n\r\n"

    def get_stream_resp_chunk_type(
        self, json_data: Dict[str, Any]
    ) -> StreamChunkDataType:
        try:
            has_content = False
            has_inline_data = False
            candidate0 = json_data["candidates"][0]
            if "content" in candidate0:
                if "parts" in candidate0["content"]:
                    part0 = candidate0["content"]["parts"][0]
                    if "text" in part0:
                        has_content = True
                    if "inline_data" in part0:
                        has_inline_data = True

            if "finishReason" in candidate0 and candidate0["finishReason"] is not None:
                if has_content:
                    return StreamChunkDataType.FINISH
                else:
                    return StreamChunkDataType.FINISH_NO_CONTENT
            else:
                if has_inline_data:
                    return StreamChunkDataType.NORMAL_BINARY
                else:
                    return StreamChunkDataType.NORMAL_TEXT
        except Exception as e:
            logger.warning(f"invalid chunk: exception: {e} json_data: {json_data}")
            return StreamChunkDataType.INVALID

    def get_num_tokens_from_body(self, body: dict) -> int:
        tokens: int = 0
        # Gemini has a special "contents" field made up of "parts" in it's body to represent messages
        # This function is for request body.
        if "contents" in body:
            messages = []
            for content in body["contents"]:
                for part in content["parts"]:
                    messages.append(part.get("text", ""))
            tokens = num_tokens_from_messages(messages)
        return tokens

    def get_model_req(self, body_jsn: dict, headers_jsn: dict) -> str:
        return headers_jsn.get("x-llm-model", "")

    def get_model_resp(self, body_jsn: dict) -> str:
        return body_jsn.get("modelVersion", "")

    def is_streaming_req(self, body_jsn, headers_jsn: dict) -> bool:
        return "x-chat-streaming" in headers_jsn

    def is_streaming_response(
        self,
        is_streaming_request: bool,
        response_headers: base_pb2.HeaderMap,
        content_type: str | None = None,
    ) -> bool:
        """
        Gemini streaming response can be in 2 different ways base of the request qs param
        When the 'alt=SSE' qs param is used, it uses SSE like the others but without then ending 'data: [DONE]' Message
        Otherwise, it just stream normal json arrays. So, we check if it's SSE first by calling the parent's function
        then just a best effort if the request is streaming request
        For HTTP/1, all streaming response must use "Transfer-Encoding: chunked" but we cannot rely on that as an
        indicator because Http/2 does not need chunk encoding to stream.
        """
        if content_type is None:
            content_type = get_content_type(response_headers)

        if content_type == "text/event-stream":
            return True

        # Gemini can stream raw json where the content_type would be just 'application/json' which
        # look just like non-streaming response. So, assume it's a stream response if the request
        # is streaming request but we actually don't support that streaming format yet. This is tracked
        # by: https://github.com/kgateway-dev/kgateway/issues/10804
        if content_type == "application/json" and is_streaming_request:
            return True

        return False

    def all_req_content(self, body: dict) -> str:
        s = ""
        for idx, content in enumerate(body.get("contents", [])):
            if "role" in content:
                s += f"role: {content['role']}:\n"
            if "parts" in content:
                for jdx, part in enumerate(content.get("parts", [])):
                    if "text" in part:
                        s += part["text"]
                    if jdx != len(part) - 1:
                        s += "\n"
                if idx != len(content) - 1:
                    s += "\n"
        return s

    def construct_request_webhook_request_body(
        self, body: dict
    ) -> webhook_api.PromptMessages:
        prompt_messages = webhook_api.PromptMessages()

        for content in body.get("contents", []):
            if "parts" in content:
                found_content = False
                for part in content.get("parts", []):
                    if "text" in part:
                        found_content = True
                        prompt_messages.messages.append(
                            webhook_api.Message(
                                role=content.get("role", ""), content=part["text"]
                            )
                        )
                        # TODO(andy): can each parts has more than one "text"? The api here doesn't explicitly forbid it:
                        #             https://ai.google.dev/api/caching#Part but is that a valid use case. The reason is that
                        #             the OpenAI api format doesn't allow multiple content texts per "message". Also, if we are
                        #             to map them back after webhook modification, it needs a bit more work.
                        #             For now, assume there would be only a "text" block per Part.
                        break
                if found_content:
                    continue

            # fall through to here if "parts" doesn't exist or contains no "text" to create a message
            # so that the number of messages will  match the original if webhook is modifying the content
            prompt_messages.messages.append(
                webhook_api.Message(role=content.get("role", ""), content="")
            )

        return prompt_messages

    def update_request_body_from_webhook(
        self, original_body: dict, webhook_modified_messages: webhook_api.PromptMessages
    ):
        contents = original_body.get("contents", [])
        if len(contents) != len(webhook_modified_messages.messages):
            logger.error(
                "webhook modified messages do not match the original messages array size!"
            )
            return

        for i, modified_message in enumerate(webhook_modified_messages.messages):
            if "role" in contents[i] and contents[i]["role"] != modified_message.role:
                logger.warning(
                    "webhook modified messages attempts to modify the role from %s to %s. ignoring.",
                    contents[i]["role"],
                    modified_message.role,
                )

            if "parts" in contents[i]:
                for part in contents[i]["parts"]:
                    if "text" in part:
                        part["text"] = modified_message.content
                        break

    def construct_response_webhook_request_body(
        self, body: dict
    ) -> webhook_api.ResponseChoices:
        response_choices = webhook_api.ResponseChoices()
        for candidate in body.get("candidates", []):
            if "content" in candidate and "parts" in candidate["content"]:
                found_content = False
                for part in candidate["content"]["parts"]:
                    if "text" in part:
                        response_choices.choices.append(
                            webhook_api.ResponseChoice(
                                message=webhook_api.Message(
                                    role=candidate["content"].get("role", ""),
                                    content=part["text"],
                                )
                            )
                        )
                        found_content = True
                        break
                if found_content:
                    continue

            # Falls through here if content is not found for the candidate
            # Still need to append a choice so the len of the array will match the original
            # if webhook modifies the content
            response_choices.choices.append(
                webhook_api.ResponseChoice(
                    message=webhook_api.Message(role="", content="")
                )
            )

        return response_choices

    def update_response_body_from_webhook(
        self,
        original_body: dict,
        webhook_modified_messages: webhook_api.ResponseChoices,
    ):
        candidates = original_body.get("candidates", [])
        if len(candidates) != len(webhook_modified_messages.choices):
            logger.error(
                "webhook modified messages do not match the original candidates array size!"
            )
            return

        for i, modified_content in enumerate(webhook_modified_messages.choices):
            if "content" not in candidates[i]:
                continue

            orig_role = candidates[i]["content"].get("role", "")
            if orig_role != modified_content.message.role:
                logger.warning(
                    "webhook modified messages attempts to modify the role from %s to %s. ignoring.",
                    orig_role,
                    modified_content.message.role,
                )

            for part in candidates[i]["content"].get("parts", []):
                if "text" in part:
                    part["text"] = modified_content.message.content
                    break

    def iterate_str_req_messages(self, body: dict, cb: Callable[[str, str], str]):
        for idx, content in enumerate(body.get("contents", [])):
            role = content.get("role", "")
            if "parts" in content:
                for jdx, part in enumerate(content.get("parts", [])):
                    body["contents"][idx]["parts"][jdx]["text"] = cb(role, part["text"])

    def iterate_str_resp_messages(self, body: dict, cb: Callable[[str, str], str]):
        for idx, candidate in enumerate(body.get("candidates", [])):
            if "content" in candidate:
                content = candidate["content"]
                role = content.get("role", "")
                if "parts" in content:
                    for jdx, part in enumerate(content["parts"]):
                        body["candidates"][idx]["content"]["parts"][jdx]["text"] = cb(
                            role, part["text"]
                        )

    def extract_contents_from_resp_chunk(self, json_data) -> List[bytes] | None:
        if json_data is None:
            return None

        # candidates[]->content->parts[]->text
        contents: List[bytes] = []
        for i, candidate in enumerate(json_data["candidates"]):
            if "content" in candidate:
                if "parts" in candidate["content"]:
                    contents.append(b"")
                    for part in candidate["content"]["parts"]:
                        if "text" in part:
                            contents[i] += part["text"].encode("utf-8")
                        else:
                            # TODO(andy): If there is a part without text, that means it might be
                            #             other data like audio, images ...etc. We probably want
                            #             to flush the fifo and pause buffering if we encounter this
                            #             so we don't buffer large amount of binary data for nothing.
                            #             but still need to consider cache
                            pass

        if len(contents) == 0:
            return None

        logger.debug(f"contents: {contents}")
        return contents

    def has_choice_index(
        self, json_data: Dict[str, Any] | None, choice_index: int
    ) -> bool:
        if json_data is None:
            logger.warning(
                f"has_choice_index() called by no json_data. choice_index: {choice_index}"
            )
            return False

        if len(json_data["candidates"]) == 0:
            logger.warning(
                f"has_choice_index() called by no candidates in json_data. choice_index: {choice_index}"
            )
            return False

        for candidate in json_data["candidates"]:
            if candidate.get("index", 0) == choice_index:
                return True

        return False

    def update_stream_resp_contents(self, json_data, choice_index: int, content: bytes):
        # Confirmed that gemini does not support multi-choices response in v1beta, it will return 400
        # if generationConfig -> candidateCount is greater than 1
        if choice_index > 1:
            logger.warning(
                "update_stream_res_contents(): gemini API does not support multi choices response"
            )
            return None

        if json_data is None or not self.has_choice_index(json_data, choice_index):
            logger.warning(
                f"update_stream_resp_contents() called but does not have choice_index: {choice_index} content: {content}"
            )
            return None

        # content is in candidates[]->content->parts[]->text
        candidate = json_data["candidates"][choice_index]
        if "content" not in candidate:
            return None

        if "parts" not in candidate["content"]:
            return None

        for part in candidate["content"]["parts"]:
            if "text" in part:
                part["text"] = content.decode("utf-8")
                break
            # TODO(andy): how do we handle multiple parts here. Note that this is not the
            #            multi-choices, this is where other data like audio, images can interleave
            #            with text which we probably don't handle it.

    def is_streaming_response_completed(
        self,
        chunk: StreamChunkData,
    ) -> bool:
        if chunk.json_data is not None:
            # Gemini SSE does not always have [DONE] message depending on the model
            candidates = chunk.json_data.get("candidates", [])
            for candidate in candidates:
                if candidate.get("finishReason", "") == "STOP":
                    return True

        return False


def num_tokens_from_messages(messages: list[dict]) -> int:
    # we pull the tiktoken encoding when building the docker image to allow
    # execution in an air-gap environment. If this encoding is changed, make sure
    # to change the projects/ai-extension/Dockerfile as well
    encoding = tiktoken.get_encoding("cl100k_base")
    tokens_per_message = 3
    tokens_per_name = 1
    num_tokens = 0

    def count_tokens(obj):
        """
        Handle token counting based on the value type
        (i.e. not everything in the LLM response will be a string).
        """
        nonlocal num_tokens
        if isinstance(obj, str):
            num_tokens += len(encoding.encode(obj))
        elif isinstance(obj, dict):
            for key, value in obj.items():
                count_tokens(key)
                count_tokens(value)
                if key == "name":
                    num_tokens += tokens_per_name
        elif isinstance(obj, list):
            for item in obj:
                count_tokens(item)

    for message in messages:
        num_tokens += tokens_per_message
        count_tokens(message)

    num_tokens += 3  # every reply is primed with <|start|>assistant<|message|>
    return num_tokens
