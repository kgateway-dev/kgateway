import httpx
import json
import logging

from guardrails.api import (
    GuardrailsPromptRequest,
    GuardrailsPromptResponse,
    GuardrailsResponseRequest,
    GuardrailsResponseResponse,
    MaskAction,
    PassAction,
    RejectAction,
    ResponseChoices,
    ResponseChoice,
    PromptMessages,
    Message,
)
from typing import Iterable, List, Tuple

logger = logging.getLogger(__name__)


class WebhookException(Exception):
    """Webhook Exception"""

    pass


async def make_request_webhook_request(
    webhook_host: str,
    webhook_port: int,
    headers: dict[str, str],
    promptMessages: PromptMessages,
) -> PromptMessages | RejectAction | None:
    req = GuardrailsPromptRequest(body=promptMessages)
    try:
        async with httpx.AsyncClient(verify=False) as client:
            response = await client.post(
                url=f"https://{webhook_host}:{webhook_port}/request",
                json=req.model_dump(),
                headers=headers,
            )
            resp = GuardrailsPromptResponse(**response.json())
            match resp.action:
                case pass_action if isinstance(pass_action, PassAction):
                    pass
                case mask_action if isinstance(mask_action, MaskAction):
                    if isinstance(mask_action.body, PromptMessages):
                        return mask_action.body
                    else:
                        logger.error(
                            "request webhook returned wrong message type %s, expecting PromptMessages",
                            type(mask_action.body),
                        )

                case reject_action if isinstance(reject_action, RejectAction):
                    return reject_action

    except json.JSONDecodeError as e:
        err_msg = (
            f"JSON decoding error occured while parsing guardrails response output: {e}"
        )
        logger.error(err_msg)
        raise WebhookException(err_msg)
    except httpx.HTTPError as e:
        err_msg = f"Request error occured while reaching out to guardrails webhook: {e}"
        logger.error(err_msg)
        raise WebhookException(err_msg)
    except Exception as e:
        err_msg = f"Unknown error with webhook occured: {e}"
        logger.error(err_msg)
        raise WebhookException(err_msg)


async def make_response_webhook_request(
    webhook_host: str, webhook_port: int, headers: dict[str, str], rc: ResponseChoices
) -> ResponseChoices | None:
    """
    This function calls the response webhook request api and return ResponseChoices
    if webhook modified the content and None if there was no modification
    """
    req = GuardrailsResponseRequest(body=rc)
    try:
        async with httpx.AsyncClient(verify=False) as client:
            response = await client.post(
                url=f"https://{webhook_host}:{webhook_port}/response",
                json=req.model_dump(),
                headers=headers,
            )
            resp = GuardrailsResponseResponse(**response.json())
            match resp.action:
                case pass_action if isinstance(pass_action, PassAction):
                    pass
                case mask_action if isinstance(mask_action, MaskAction):
                    if isinstance(mask_action.body, ResponseChoices):
                        return mask_action.body
                    else:
                        logger.error(
                            "response webhook returned wrong message type %s, expecting ResponseChoices",
                            type(mask_action.body),
                        )
                # GuardrailsResponseResponse.action doesn't actually allow reject_action
                # case reject_action if isinstance(reject_action, RejectAction):
                #     pass
    except json.JSONDecodeError as e:
        err_msg = (
            f"JSON decoding error occured while parsing guardrails response output: {e}"
        )
        logger.error(err_msg)
        raise WebhookException(err_msg)
    except httpx.HTTPError as e:
        err_msg = f"Request error occured while reaching out to guardrails webhook: {e}"
        logger.error(err_msg)
        raise WebhookException(err_msg)
    except Exception as e:
        err_msg = f"Unknown error with webhook occured: {e}"
        logger.error(err_msg)
        raise WebhookException(err_msg)


def extract_contents_from_response_webhook_response(rc: ResponseChoices) -> List[str]:
    contents = []
    for choice in rc.choices:
        contents.append(choice.message.content)

    return contents


async def call_response_webhook(
    webhook_host: str,
    webhook_port: int,
    headers: dict[str, str],
    contents: Iterable[str],
) -> Tuple[bool, List[str] | None]:
    """
    This function is used to construct the response webhook request for streaming response
    when the buffer has enough content. This will be called multiple times per response from
    the LLM as the buffer fill up with enough contents
    """
    rc = ResponseChoices()
    for i, content in enumerate(contents):
        rc.choices.append(ResponseChoice(message=Message(role="", content=content)))

    response = await make_response_webhook_request(
        webhook_host=webhook_host, webhook_port=webhook_port, headers=headers, rc=rc
    )

    if response is None:
        return False, None

    return True, extract_contents_from_response_webhook_response(response)
