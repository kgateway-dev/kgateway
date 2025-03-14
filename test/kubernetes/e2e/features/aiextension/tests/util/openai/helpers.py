import logging

from typing import Tuple, List
from openai import OpenAI, AzureOpenAI, Stream
from openai.types.chat.chat_completion_chunk import ChatCompletionChunk
from openai.types.chat.chat_completion import ChatCompletion

logger = logging.getLogger(__name__)


def make_request(
    client: OpenAI | AzureOpenAI,
    instruction: str,
    stream: bool = False,
    n: int = 1,
    model: str = "gpt-4o-mini",
) -> ChatCompletion | Stream[ChatCompletionChunk]:
    return client.chat.completions.create(
        model=model,
        messages=[
            {
                "role": "user",
                "content": instruction,
            }
        ],
        stream=stream,
        stream_options={"include_usage": True} if stream else {},
        n=n,
    )


def count_pattern_and_extract_data_in_chunks(
    resp: Stream[ChatCompletionChunk], pattern: str, choice_index: int
) -> Tuple[int, str, int, int]:
    """
    This helper search for a pattern in all the chunks from a specific choice_index
    resp is a Stream, once it's iterated once, it would be empty, so this function only support extracting
    from one choice and the same resp cannot be used again
    While iterating through the chunks, it also extract other data. The return values in the tuple:
        pattern_match_count: int
        complete_response: str - concatenate all the contents for that single choice_index into a single string
        prompt_tokens: int
        completion_tokens: int
    """
    logger.debug(
        f"count_pattern_and_extract_data_in_chunks(): pattern={pattern}, choice_index={choice_index}"
    )
    count = 0
    complete_response = ""
    prompt_tokens = 0
    completion_tokens = 0
    for chunk in resp:
        assert chunk is not None
        if chunk.usage is not None:
            # for OpenAI, the token is only in the last chunk but if they do appear in all chunks,
            # the prompt token would be repeated, so it's `=` and not `+=`
            prompt_tokens = chunk.usage.prompt_tokens
            completion_tokens += chunk.usage.completion_tokens
        if len(chunk.choices) < 1:
            logger.debug("chunk has 0 choices")
            continue
        if chunk.choices[0].index != choice_index:
            continue
        content = chunk.choices[0].delta.content
        if content is not None:
            #            logger.debug(f"content: {content}")
            complete_response += content
            if content.find(pattern) >= 0:
                count += 1
        else:
            logger.debug("content is None")

    return count, complete_response, prompt_tokens, completion_tokens


def extract_all_choices_in_chunks(
    resp: Stream[ChatCompletionChunk],
) -> Tuple[List[str], int, int]:
    contents = [""]
    prompt_tokens = 0
    completion_tokens = 0
    for chunk in resp:
        assert chunk is not None
        if chunk.usage is not None:
            # for OpenAI, the token is only in the last chunk but if they do appear in all chunks,
            # the prompt token would be repeated, so it's `=` and not `+=`
            prompt_tokens = chunk.usage.prompt_tokens
            completion_tokens += chunk.usage.completion_tokens
        if len(chunk.choices) < 1:
            logger.debug("chunk has 0 choices")
            continue
        choice_index = chunk.choices[0].index
        # Make sure contents is large enough to hold the choice_index. The multiplication can be negative
        # and it will be no-op
        contents.extend([""] * (choice_index + 1 - len(contents)))
        content = chunk.choices[0].delta.content
        if content is not None:
            #            logger.debug(f"content: {content}")
            contents[choice_index] += content
        else:
            logger.debug("content is None")

    return contents, prompt_tokens, completion_tokens
