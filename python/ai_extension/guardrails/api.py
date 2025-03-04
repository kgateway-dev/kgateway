import yaml

from pydantic import BaseModel, Field
from typing import List
from pydantic.json_schema import models_json_schema


### Guardrails API ###
# The following classes are used to define the body field in the request and response models for
# the Guardrails API.
class Message(BaseModel):
    """
    Message contains the text prompts for a request to the LLM or the response content from the LLM.
    """

    role: str = Field(
        description="The role associated to the content in this message.",
        examples=["Assistant"],
    )

    content: str = Field(
        description="The content text for this message.",
        examples=["How can I help you today?"],
    )


class PromptMessages(BaseModel):
    """
    PromptMessages contains a list of prompt messages in a request to the LLM
    """

    messages: List[Message] = Field(
        default_factory=list,
        description="List of prompt messages including role and content.",
        examples=[
            [
                {"role": "Assistant", "content": "How can I help you?"},
                {"role": "User", "content": "What is 1 + 2?"},
            ]
        ],
    )


class ResponseChoice(BaseModel):
    """
    ResponseChoice is a single choice of the chat completion response text from the LLM
    """

    message: Message = Field(
        description="message contains the role and text content of the response from the LLM model.",
        examples=[{"role": "Assistant", "content": "1 + 2 is 3"}],
    )


class ResponseChoices(BaseModel):
    """
    ResponseChoices contains a list of response choices from the LLM. Each choice represent a separate independent response.
    """

    choices: List[ResponseChoice] = Field(
        default_factory=list,
        description="list of possible independent responses from the LLM",
        examples=[
            [
                {"message": {"role": "Assistant", "content": "1 + 2 is 3"}},
                {
                    "message": {
                        "role": "Assistant",
                        "content": "The result of adding 1 to 2 is 3",
                    }
                },
            ]
        ],
    )


# The following classes are used to define the request and response models for the Guardrails API.


class GuardrailsPromptRequest(BaseModel):
    """
    GuardrailsPromptRequest is the request model for the Guardrails prompt API.
    """

    body: PromptMessages = Field(
        description="body contains the object which is a list of the Message JSON objects from the prompts in the request"
    )


class MaskAction(BaseModel):
    """
    MaskAction is the response model for the Mask action which indicates the message has been modified.
    This can be used in GuardrailsPromptResponse or GuardrailsResponseResponse when responding to a GuardrailsPromptRequest or a GuardrailsResponseRequest respectively
    """

    body: PromptMessages | ResponseChoices = Field(
        description="body contains the modified messages that masked out some of the original contents. When used in a GuardrailPromptResponse, this should be PromptMessages. When used in GuardrailResponseResponse, this should be ResponseChoices"
    )

    reason: str | None = Field(
        description="reason is a human readable string that explains the reason for the action.",
        default=None,
    )


class RejectAction(BaseModel):
    """
    RejectAction is the response model for the Reject action which indicate the request should be rejected.
    This action will cause a HTTP error response to be sent back to the end-user.
    """

    body: str = Field(
        description="body is the rejection message that will be used for HTTP error response body."
    )

    status_code: int = Field(
        description="status_code is the HTTP status code to be returned in the HTTP error response."
    )

    reason: str | None = Field(
        description="reason is a human readable string that explains the reason for the action.",
        default=None,
    )


class PassAction(BaseModel):
    """
    PassAction is the response model for the Pass action which indicate no modification is done to the messages.
    """

    reason: str | None = Field(
        description="reason is a human readable string that explains the reason for the action.",
        default=None,
    )


class GuardrailsPromptResponse(BaseModel):
    """
    GuardrailsPromptResponse is the response model for the Guardrails prompt API.
    """

    action: PassAction | MaskAction | RejectAction = Field(
        description="""
        action is the action to be taken based on the request.
        The following actions are available on the response:
        - PassAction: No action is required.
        - MaskAction: Mask the response body.
        - RejectAction: Reject the request.
        """
    )


class GuardrailsResponseRequest(BaseModel):
    """
    GuardrailsResponseRequest is the request model for the Guardrails response API.
    """

    body: ResponseChoices = Field(
        description="body contains the object with a list of Choice that contains the response content from the LLM."
    )


class GuardrailsResponseResponse(BaseModel):
    """
    GuardrailsResponseResponse is the response model for the Guardrails response API.
    """

    action: PassAction | MaskAction = Field(
        description="""
        action is the action to be taken based on the request.
        The following actions are available on the response:
        - PassAction: No action is required.
        - MaskAction: Mask the response body.
        """
    )


def print_json_schema(models):
    _, schemas = models_json_schema(
        [(model, "validation") for model in models],
        ref_template="#/components/schemas/{model}",
    )
    openapi_schema = {
        "openapi": "3.1.0",
        "info": {
            "title": "Gloo AI Gateway Guardrails Webhook API",
            "version": "0.0.1",
        },
        "components": {
            "schemas": schemas.get("$defs"),
        },
    }
    print(yaml.dump(openapi_schema, sort_keys=False))


if __name__ == "__main__":
    print_json_schema(
        [
            Message,
            PromptMessages,
            ResponseChoice,
            ResponseChoices,
            MaskAction,
            RejectAction,
            PassAction,
            GuardrailsPromptRequest,
            GuardrailsPromptResponse,
            GuardrailsResponseRequest,
            GuardrailsResponseResponse,
        ]
    )
