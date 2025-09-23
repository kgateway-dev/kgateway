from copy import deepcopy
import logging
import re
import traceback

from api.kgateway.policy.ai import prompt_guard
from api.envoy.service.ext_proc.v3 import external_processor_pb2
from collections import deque
from dataclasses import dataclass
from ext_proc.provider import Provider, Tokens
from ext_proc.streamchunkdata import StreamChunkData, StreamChunkDataType


# from ext_proc.stream import Handler
from opentelemetry import trace
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from presidio_analyzer import EntityRecognizer
from presidio_anonymizer import AnonymizerEngine
from telemetry.tracing import OtelTracer
from typing import Any, Callable, Deque, Dict, List, Tuple
from guardrails.regex import regex_transform
from guardrails.webhook import call_response_webhook
from util import sse

logger = logging.getLogger().getChild("kgateway-ai-ext.streamchunks")


def reconstruct_chunk(llm_provider: Provider, chunk: StreamChunkData):
    """
    reconstruct the contents and raw_data inside the chunk base on the json_data inside the chunk
    """

    # chunk.json_data can be None here which will clear out the contents in the chunk
    chunk.set_contents(llm_provider.extract_contents_from_resp_chunk(chunk.json_data))

    if chunk.json_data is None:
        return

    try:
        chunk.raw_data = sse.replace_json_data(
            raw_data=chunk.raw_data, json_data=chunk.json_data
        )
    except sse.SSEParsingException as e:
        logger.error(f"reconstruct_chunk: failed to replace json data: {e}")

    return


def update_chunk_content(
    llm_provider: Provider, chunk: StreamChunkData, choice_index: int, content: bytes
):
    """
    update_chunk_content() locks the two steps together to make sure the data are consistent in the structure:
        1) update the the content in json_data
        2) update the raw_data and other fields base on json_data
    """
    llm_provider.update_stream_resp_contents(
        json_data=chunk.json_data, choice_index=choice_index, content=content
    )
    # Update the raw_data and content of the chunk and reconstruct the contents to maintain consistency
    reconstruct_chunk(llm_provider, chunk)

    return


def update_chunk_usage(llm_provider: Provider, chunk: StreamChunkData, usages: Tokens):
    """
    update_chunk_usage() locks the two steps together to make sure the data are consistent in the structure:
        1) update the usage data in json_data
        2) update the raw_data and other fields base on json_data
    """
    if chunk.json_data is None:
        logger.warning("update_chunk_usage: chunk has no json data")
        return

    llm_provider.update_stream_resp_usage_token(chunk.json_data, usages)
    reconstruct_chunk(llm_provider, chunk)

    return


def split_chunk_content_by_boundary_indicator(
    llm_provider: Provider,
    chunk: StreamChunkData,
    boundary_indicator: bytes,
    choice_index: int,
) -> bytes:
    """
    split_chunk_content_by_boundary_indicator() is used to handle 3 out of the 5 scenarios base on where
    the boundary is located. The full list of scenarios can be found inside the align_contents_for_guardrail()
    function. 1) and 2) are not here because there is not splitting needed.

    returns the bytes that have been stripped out of this chunk
    """
    # Scenarios:
    # 3) the full boundary indicator are at the beginning of this chunk
    # 4) the whole boundary indicator are in the middle of this chunk
    # 5) boundary indicator is split between 2 or more chunks and part of the bytes
    #    are at the beginning of this chunk
    bytes_stripped = b""
    num_bytes_to_strip = 0
    chunk_content = chunk.get_content(choice_index)
    if chunk_content.startswith(boundary_indicator):
        # This is scenario #3, we will strip off the entire boundary_indicator from this chunk
        # and append that to the prev chunk
        bytes_stripped = boundary_indicator
        num_bytes_to_strip = len(boundary_indicator)
    else:
        pos = chunk_content.rfind(boundary_indicator)
        if pos >= 0:
            # This is scenario #4, we need to find where the boundary_indicator is in this chunk, move
            # the beginning of this chunk (including the boundary_indicator) to the prev chunk
            end_pos = pos + len(boundary_indicator)
            bytes_stripped = chunk_content[0:end_pos]
            num_bytes_to_strip = end_pos
        else:
            # This is scenario #5, we will strip off the part of boundary_indicator that are at the beginning
            # of this chunk. In this scenario, the boundary indicator spans across multiple chunks, so we
            # need to loop through the bytes in the boundary indicator and see which part of it is
            # at the beginning of the current chunk content

            # This scenario has to be checked last because when we split the matchBytes, we might
            # get false positive. For example, if the boundary_indicator is b'. ' and chunk content is
            # " simple server. Envoy is ", when we search for ' ' after dropping the first char
            # from boundary_indicator, it matches the beginning but it's wrong

            for offset in range(1, len(boundary_indicator)):
                partialMatchedBytes = boundary_indicator[offset:]
                if chunk_content.startswith(partialMatchedBytes):
                    bytes_stripped = partialMatchedBytes
                    num_bytes_to_strip = len(partialMatchedBytes)
                    break

    update_chunk_content(
        llm_provider,
        chunk,
        choice_index=choice_index,
        content=chunk_content[num_bytes_to_strip:],
    )

    return bytes_stripped


@dataclass
class StreamChunksContent:
    """
    StreamChunksContent hold the content as a str and the begin index (inclusive) and end index
    (exclusive) of the chunks in the streaming_fifo that make up the content string
    """

    content: str = ""
    begin_index: int = -1
    end_index: int = -1


def find_max_min_end_index(
    stream_contents: List[StreamChunksContent],
) -> Tuple[int, int]:
    """
    returns the highest and lowest end index
    """
    highest_end_index = max(stream_contents, key=lambda item: item.end_index).end_index
    lowest_end_index = min(stream_contents, key=lambda item: item.end_index).end_index

    return highest_end_index, lowest_end_index


@dataclass
class BoundaryMatchData:
    """
    BoundaryMatchData is a dataclass that store the captured pattern and start and end
    position from a regex match
    """

    choice_index: int = -1
    """
    The index into the contents array in SteamChunks when multi-choice response is used
    """

    capture: str = ""
    """
    The string matching the capture group pattern. An empty string here means there was no match
    """

    start_pos: int = -1
    """
    The start position where capture appears on the original string (inclusive)
    """

    end_pos: int = -1
    """
    The end position where capture ends on the original string (exclusive)
    """


class StreamChunks:
    __total_bytes_buffered: int = 0
    """
    This is the regex pattern to detect segment boundary in content message so we can send a complete
    segment to webhook or regex match.
    """

    def __init__(self):
        self.__streaming_fifo: Deque["StreamChunkData"] = deque()
        """
        streaming_fifo is used to buffer the content if handler.resp_webhook or handler.resp_regex
        is set. The data the come out of this FIFO will be append to body for caching later.
        This is not used for request.
        """

        self.__contents: List[bytearray] = list()
        """
        The content messages in the streaming fifo. Each entry contains the message for choice i where i is the index into the array
        """

        self.__roles: List[str] = list()
        """
        The role of the messages in the streaming fifo. Each entry contains the role for the message for choice i where i is the index
        into the array and correspond to the "index" field in the "choices" array in the json data. OpenAI only send the role in the
        first chunk with empty content. So, we store it here in case we need it 
        """

        self.__leftover: bytes = bytes()
        """
        The leftover bytes of any incomplete chunks from the previous buffer() call
        """

        self.model: str = ""
        """
        The model from the streaming response
        """

        self.tokens: Tokens | None = None
        """
        Accumulated tokens for the response stream
        """

        self.is_function_calling: bool = False
        """
        Indicate if this streaming response is a function calling response
        """

        self.is_completed: bool = False
        """
        This indicated the stream is completed properly base on the provider specific indicator
        """

    def get_role(self, choice_index: int) -> str:
        if choice_index < 0 or choice_index >= len(self.__roles):
            return ""

        return self.__roles[choice_index]

    def set_role(self, choice_index: int, role: str):
        if choice_index < 0:
            return

        size = len(self.__roles)
        self.__roles.extend([""] * (choice_index + 1 - size))
        self.__roles[choice_index] = role

    def get_contents_with_chunk_indices(self) -> List[StreamChunksContent]:
        contents: List[StreamChunksContent] = list()
        for item in self.__contents:
            contents.append(
                StreamChunksContent(
                    content=item.decode("utf-8"),
                    begin_index=0,
                    end_index=len(self.__streaming_fifo),
                )
            )
        return contents

    def reconstruct_contents(self):
        for item in self.__contents:
            item.clear()

        for chunk in self.__streaming_fifo:
            chunk_contents = chunk.get_contents()
            if chunk_contents is None:
                continue
            for choice_index, content in enumerate(chunk_contents):
                self.__contents[choice_index].extend(content)

    def adjust_contents_size(self, chunk: StreamChunkData):
        """
        adjust_contents_size() expands the size of self.__contents list as needed so it will be big enough to store
        the incoming contents.
        For non-multichoice response, the size of the contents list is always 1.
        """
        chunk_contents = chunk.get_contents()
        if chunk_contents is None:
            return

        current_contents_len = len(self.__contents)
        incoming_contents_len = len(chunk_contents)
        self.__contents.extend(
            [bytearray()] * (incoming_contents_len - current_contents_len)
        )

    def append_chunk(self, chunk: StreamChunkData):
        """
        append one chunk to the end of streaming_fifo, update contents and do the necessary accounting
        """
        self.__streaming_fifo.append(chunk)
        StreamChunks.__total_bytes_buffered += len(chunk.raw_data)
        logger.debug(f"append_chunk(): chunk: {chunk}")
        chunk_contents = chunk.get_contents()
        if chunk_contents is not None:
            self.adjust_contents_size(chunk)

            logger.debug(f"append_chunk(): chunk_contents: {chunk_contents}")
            for i, content in enumerate(chunk_contents):
                logger.debug(f"append_chunk(): i: {i} content: {content}")
                self.__contents[i].extend(content)

    def pop_chunk(self) -> StreamChunkData | None:
        """
        pop one chunk from the head of streaming_fifo, update contents and do the necessary accounting
        """
        if len(self.__streaming_fifo) == 0:
            return None

        chunk = self.__streaming_fifo.popleft()
        StreamChunks.__total_bytes_buffered -= len(chunk.raw_data)
        chunk_contents = chunk.get_contents()
        if chunk_contents is not None:
            for i, content in enumerate(chunk_contents):
                if self.__contents[i].startswith(content):
                    del self.__contents[i][: len(content)]
                else:
                    logger.critical(
                        f'"{self.__contents[i]}" does not start with "{content}"'
                    )

        return chunk

    def pop_all(self) -> bytes:
        """
        pop_all pops all the chunks in streaming_filo and return all the raw_data in a single bytes object
        """
        raw_data = bytearray()
        for i, chunk in enumerate(self.__streaming_fifo):
            StreamChunks.__total_bytes_buffered -= len(chunk.raw_data)
            logger.debug(
                f"pop_all: i: {i} total_bytes_buffered: {StreamChunks.__total_bytes_buffered} raw_data: {chunk.raw_data}"
            )
            raw_data.extend(chunk.raw_data)
        self.__contents.clear()
        self.__streaming_fifo.clear()
        return bytes(raw_data)

    def pop_chunks(self, n: int) -> bytes | None:
        """
        pop_chunks pops n chunks from streaming_fifo and returns the raw_data in those chunks in a single bytes object
        if n is greater or equal to the size of streaming_fifo, it would just call pop_all()
        """

        if n <= 0:
            return None

        if n >= len(self.__streaming_fifo):
            return self.pop_all()

        raw_data = bytearray()
        for i in range(n):
            chunk = self.pop_chunk()
            if chunk is not None:
                # because we already check size above, chunk should never be None
                raw_data.extend(chunk.raw_data)
                logger.debug(f"pop_chunks: n: {n} i: {i} raw_data: {chunk.raw_data}")
            else:
                logger.debug(f"pop_chunks: n: {n} i: {i} chunk is None")
        return bytes(raw_data)

    def delete_chunks(self, start_index: int, end_index: int):
        """
        delete_chunks() delete the chunks from start_index to end_index (exclusive).
        """
        logger.debug(
            f"delete_chunks(): start: {start_index} end: {end_index} fifo size: {len(self.__streaming_fifo)}"
        )

        if end_index > len(self.__streaming_fifo) or start_index < 0:
            logger.critical(
                "delete_chunks() out of bound: start_index: {} end_index: {} fifo size: {}",
                start_index,
                end_index,
                len(self.__streaming_fifo),
            )

        num_chunks_to_delete = end_index - start_index
        for _ in range(num_chunks_to_delete):
            del self.__streaming_fifo[start_index]

    def has_min_chunks_with_contents(self, min: int) -> bool:
        count = 0
        for chunk in self.__streaming_fifo:
            if (
                chunk.type == StreamChunkDataType.NORMAL_TEXT
                or chunk.type == StreamChunkDataType.FINISH
            ):
                count += 1
                if count >= min:
                    return True

        return False

    def find_chunk_with_boundary_indicator(
        self, match: BoundaryMatchData, stream_content_data: StreamChunksContent
    ) -> int:
        """
        returns the chunk index that contains the end of the boundary indicator. -1 if not found
        stream_content_data is the entire content for the current buffered stream for 1 particular choice.
        """
        logger.debug(f"match.choice_index: {match.choice_index}")
        bytes_count_from_end = len(stream_content_data.content) - match.end_pos
        for chunk_reverse_index, chunk in enumerate(reversed(self.__streaming_fifo)):
            chunk_contents = chunk.get_contents()
            if chunk_contents is None:
                # chunk_contents can be None for chunks that has no content field. eg the last chunk
                # sometimes can contain of the the stop reason but no content
                continue
            logger.debug(
                f"align: 1.1 chunk {chunk} bytes_count_from_end: {bytes_count_from_end}"
            )
            content_len = chunk.get_content_length(match.choice_index)
            bytes_count_from_end -= content_len
            logger.debug(
                f"reverse_index: {chunk_reverse_index}, content_len: {content_len}, bytes_count_from_end: {bytes_count_from_end}"
            )
            if bytes_count_from_end < 0:
                # we found the chunk that contains the end of the matched capture
                chunk_index = len(self.__streaming_fifo) - 1 - chunk_reverse_index
                return chunk_index

        return -1

    def find_prev_chunk_containing_choice(
        self, llm_provider: Provider, current_chunk_index: int, choice_index: int
    ) -> Tuple[int, StreamChunkData]:
        """
        find the chunk(going from current_chunk_index backward on self.__stream_fifo) that
        contains content for the specified choice_index and
        returns the chunk_index and the chunk that contains content for the specif
        """
        prev_chunk_index = current_chunk_index - 1
        prev_chunk = self.__streaming_fifo[prev_chunk_index]
        # Because OpenAI put 1 choice per chunk, so need to iterate
        # backward until we find the chunk that has the same choice_index
        while (
            not llm_provider.has_choice_index(prev_chunk.json_data, choice_index)
            and prev_chunk_index >= 0
        ):
            prev_chunk_index -= 1
            prev_chunk = self.__streaming_fifo[prev_chunk_index]

        return prev_chunk_index, prev_chunk

    def align_contents_for_guardrail(
        self,
        llm_provider: Provider,
        stream_contents: List[StreamChunksContent],
        min_content_length: int,
    ) -> int:
        """
        find the segment boundary in each content and adjust the content text and chunk indices
        re-align the chunks in the fifo if necessary to make all the end indices on the same chunk
        returns how many chunks should be popped out to send back to envoy
        """
        # You can find a multi choice example for this logic in
        # test_align_contents_for_guardrail_multichoices in test_streamchunks.py.

        min_chunks_required = 2
        if len(stream_contents) > 1:
            # This is because for OpenAI, each chunk contains 1 choice, so if we have more choices,
            # whe need more chunks to have all the choices buffered
            min_chunks_required *= len(stream_contents)

        if not self.has_min_chunks_with_contents(min_chunks_required):
            # We need at least 2 chunks that have contents to move things around
            # For the end_of_steam scenario where we may not have 2 chunks, this function
            # should not be called as we just send all contents to guardrails at that point
            return 0

        match_list = self.find_segment_boundary(stream_contents)
        if len(match_list) != len(stream_contents):
            # This means we don't have a match or we have multi-choice response and not
            # every content in every choice has a match.
            # We wait until we have a match in every content and collapse the chunks
            # before webhook.
            return 0

        logger.debug(
            f"align_contents_for_guardrail: stream_contents {stream_contents} match_list {match_list}"
        )
        for match in match_list:
            if match.end_pos < min_content_length:
                logger.debug(
                    f"align: 0.1 boundary too short: stream_contents {stream_contents} match_list {match_list}"
                )
                return 0

        for match in match_list:
            # each match corresponds to one choice in the response
            # find which chunk(s) contains the matched capture. Starts from the end of the fifo
            # when found, split the content in the chunk so everything before and including the boundary
            # indicator will be moved to the chunk before this chunk in the fifo
            stream_content_data = stream_contents[match.choice_index]
            chunk_index = self.find_chunk_with_boundary_indicator(
                match, stream_content_data
            )

            if chunk_index < 0:
                # This should never happen
                logger.warning(
                    "align_contents_for_guardrail: cannot find chunk with boundary indicator"
                )
                logger.debug(
                    f"match: {match} stream_content_data: {stream_content_data}"
                )
                continue

            # Now that we found the chunk the contains the end of the boundary indictor.
            # Adjust the stream_content_data to strip out everything after the boundary indicator.
            # The content in stream_content_data is the content that will be passed to guard rail
            # (webhook or regex match)
            stream_content_data.end_index = chunk_index
            stream_content_data.content = stream_content_data.content[: match.end_pos]

            # Then, we want to split the chunk_content to keep what's after the boundary indictor
            # in the chunk but move everything else in the chunk that's before this chunk
            # in the fifo. So, when we pop out the chunks to send back to envoy, the stream_contents
            # matched what have already went through guard rail
            chunk = self.__streaming_fifo[chunk_index]

            # There are 5 scenarios where the boundary_indicator can be:
            # 1) the whole boundary_indicator are at the end of this chunk
            # 2) the boundary_indicator are longer then this chunk and this whole chunk is the last part of boundary_indicator
            # 3) the full boundary_indicator are at the beginning of this chunk
            # 4) the whole boundary_indicator are in the middle of this chunk
            # 5) split between 2 or more chunks and part of the bytes are at the beginning of this chunk

            chunk_content = chunk.get_content(match.choice_index)
            boundary_indicator = match.capture.encode("utf-8")
            if chunk_content.endswith(boundary_indicator) or (
                len(chunk_content) < len(boundary_indicator)
                and boundary_indicator.endswith(chunk_content)
            ):
                # This is scenario #1 and #2, no alignment needs to be done
                # Increase the end_index so this chunk is included for guard rail
                stream_content_data.end_index += 1
                break

            bytes_to_append_to_prev_chunk = split_chunk_content_by_boundary_indicator(
                llm_provider,
                chunk,
                boundary_indicator,
                match.choice_index,
            )

            # For the chunk in front of the current chunk, we append the bytes
            # we have stripped off from the current chunk
            # Because OpenAI put 1 choice per chunk, so need to find the chunk that also
            # has the content for the same choice
            if chunk_index >= 1:
                _, prev_chunk = self.find_prev_chunk_containing_choice(
                    llm_provider, chunk_index, match.choice_index
                )
                if prev_chunk.get_contents() is not None:
                    new_content = (
                        prev_chunk.get_content(match.choice_index)
                        + bytes_to_append_to_prev_chunk
                    )
                    update_chunk_content(
                        llm_provider,
                        prev_chunk,
                        choice_index=match.choice_index,
                        content=new_content,
                    )
            else:
                logger.critical(
                    "align_contents_for_guardrail: chunk index reached 0 unexpectedly"
                )

        if len(stream_contents) > 1:
            # Handle more than 1 choice. Align the chunks if the boundary for different
            # choices are at different chunk_index
            highest_end_index, lowest_end_index = find_max_min_end_index(
                stream_contents
            )
            if lowest_end_index != highest_end_index:
                # end indices are not aligned, need to re-align the chunks
                # so all the contents are before the lowest_end_index chunk
                for choice_index, content_data in enumerate(stream_contents):
                    if content_data.end_index > lowest_end_index:
                        dst_chunk_index, _ = self.find_prev_chunk_containing_choice(
                            llm_provider, lowest_end_index, choice_index
                        )
                        # move contents before and including the boundary to the chunk before lowest_end_index
                        self.collapse_contents(
                            llm_provider,
                            choice_index,
                            lowest_end_index,
                            content_data.end_index,
                            dst_chunk_index,
                        )
                        content_data.end_index = lowest_end_index

            # Some chunks that we have moved contents away from might have empty content but they are still
            # a valid chunk with all the other fields intact and can be sent out to the user.
            # Leave them there to avoid having to move content to chunks that may not arrived yet because OpenAI
            # does not put all choices in one chunk, we may not have the chunk that contains the next content
            # of a particular choice index

        # at this point, all chunks content should be aligned and all contents should point to the same
        # lowest_end_index, so returning the end_index of the first choice as the number of chunks to pop
        # out to return to envoy for delivery
        return stream_contents[0].end_index

    def get_usage_from_chunks(
        self,
        extract_usage_func: Callable[[Dict[str, Any]], Tokens],
        start_index: int,
        end_index: int,
    ) -> Tokens:
        total = Tokens()
        for i in range(start_index, end_index):
            json_data = self.__streaming_fifo[i].json_data
            if json_data is None:
                continue

            chunk_tokens = extract_usage_func(json_data)

            # For gemini, the prompt token is repeated in every chunk, so we should not add them up
            if total.prompt == 0:
                total.prompt = chunk_tokens.prompt

            total.completion += chunk_tokens.completion

        # For OpenAI, the usage token is null in the chunk, so the total prompt and completion token would be 0
        return total

    def collapse_chunks_with_new_content(
        self,
        llm_provider: Provider,
        original_contents: List[StreamChunksContent],
        new_contents: List[str],
    ) -> int:
        """
        delete the first n - 1 chunks we are collapsing and then update the 1st chunk with the new content
        return the number of chunks we should pop out. Usually it would be 1 but if we are at the end of stream,
        the last few chunks might be a FINISH_NO_CONTENT or DONE chunks that we want to preserve as is and don't
        collapse them.
        """

        # all contents should have the same begin and end index and begin_index of every content should be 0,
        logger.debug(f"regex: collapse_chunks: {original_contents}")
        fifo_size = len(self.__streaming_fifo)
        logger.debug(
            f"regex: collapse_chunks fifo (size={fifo_size}): {self.__streaming_fifo}"
        )
        end_index = original_contents[0].end_index
        begin_index = original_contents[0].begin_index

        if end_index > fifo_size:
            # This should not happen
            logger.critical(
                f"collapse_chunks_with_new_contents: end_index out of bound. end_index: {end_index} fifo_size: {fifo_size}"
            )
            return 0

        if begin_index != 0:
            # This should not happen
            logger.critical(
                f"collapse_chunks_with_new_contents: begin_index {begin_index} is not 0 fifo_size: {fifo_size}"
            )
            return 0

        if begin_index == end_index:
            # This should not happen
            logger.critical(
                f"collapse_chunks_with_new_contents: begin_index {begin_index} is same as end_index: {end_index}"
            )
            return 0

        chunks_to_pop = 0
        # skip any non-NORMAL_TEXT chunk from the end
        for i in range(end_index - 1, begin_index, -1):
            logger.debug(f"collapse_chunks_with_new_contents: i = {i}")
            if (
                self.__streaming_fifo[i].type == StreamChunkDataType.NORMAL_TEXT
                or self.__streaming_fifo[i].type
                == StreamChunkDataType.FINISH  # FINISH type can have normal content
            ):
                break

            chunks_to_pop += 1
            end_index -= 1

        # figure out how many chunks from the beginning we should be preserving
        # because OpenAI only has 1 choice per chunk, the idea here is that we preserve
        # the consecutive blocks of chunks that has 1 chunk with non-empty content for each choice
        found_count = 0
        choice_chunk_index_list = list()
        choice_chunk_index_list.extend([-1] * len(new_contents))
        while begin_index < end_index and found_count < len(choice_chunk_index_list):
            chunk = self.__streaming_fifo[begin_index]
            for choice_index, _ in enumerate(new_contents):
                if len(chunk.get_content(choice_index)) > 0:
                    if choice_chunk_index_list[choice_index] < 0:
                        found_count += 1
                        choice_chunk_index_list[choice_index] = begin_index
                    else:
                        # If we see more than 1 chunk that contains content for the same
                        # choice, we need to clear the content so the content won't be
                        # duplicated because we are setting the new content on to the
                        # first found chunk within this consecutive block already.
                        # Normally, this should not happen but just in case
                        update_chunk_content(
                            llm_provider,
                            chunk,
                            choice_index=choice_index,
                            content=b"",
                        )
            begin_index += 1
            chunks_to_pop += 1

        # we are getting the total usage for the first num_chunks_to_collapse, so this will
        # include the usage of the chunk that we will keep for storing the new contents
        usages = self.get_usage_from_chunks(llm_provider.tokens, begin_index, end_index)

        self.delete_chunks(begin_index, end_index)

        for choice_index, content in enumerate(new_contents):
            chunk_index = choice_chunk_index_list[choice_index]
            chunk = self.__streaming_fifo[chunk_index]
            update_chunk_content(
                llm_provider,
                chunk,
                choice_index=choice_index,
                content=content.encode("utf-8"),
            )

        usage_chunk = self.__streaming_fifo[begin_index - 1]
        if usages.completion > 0 and usages.prompt > 0:
            # only set the token if both are non-zero because for OpenAI, the usage object is null and
            # will appear as 0. We don't want to change the null to 0. For others, they should not be 0
            # but if they are, that means we don't need to change it because the chunk is already 0
            update_chunk_usage(llm_provider, usage_chunk, usages)

        self.reconstruct_contents()

        logger.debug(f"collapse_chunks_with_new_content: returning {chunks_to_pop}")
        return chunks_to_pop

    async def do_guardrails_check(
        self,
        llm_provider: Provider,
        resp_headers: dict[str, str],
        webhook: prompt_guard.Webhook | None,
        regex: list[EntityRecognizer] | None,
        anonymizer_engine: AnonymizerEngine,
        parent_span: trace.Span,
        final: bool = False,
    ) -> int:
        """
        return how many chunks we should pop out from the fifo. 0 means we are just buffering until we get enough.
        """
        # this contents is a copy and will be modified locally
        contents = self.get_contents_with_chunk_indices()
        logger.debug(
            f"webhook (fifo: {len(self.__streaming_fifo)} final={final}): {contents}"
        )

        # TODO(andy): This is deviated from the original design that use a minimum chunks (mainly for simplicity)
        #             but turns out using minimum chunks do not work well with gemini because it packs a lot of
        #             tokens in a single chunk where OpenAI packs only a few tokens at most per chunk.
        #             According to ChatGPT, the average sentence for a chat is around 25 to 75 characters long,
        #             so picking 50 here. Do we need this to be configurable and get this from x-resp-guardrails-config?
        min_content_length = 50
        should_do_guardrails_check = True
        for content_data in contents:
            if len(content_data.content) < min_content_length:
                should_do_guardrails_check = False
                break

        if not final and not should_do_guardrails_check:
            # buffer the minimum before we do any guardrails check unless this is the final check (end of stream)
            return 0

        chunks_to_pop_out = len(self.__streaming_fifo)
        if not final:
            chunks_to_pop_out = self.align_contents_for_guardrail(
                llm_provider, contents, min_content_length
            )
        else:
            # if it's final, send all contents as is to guardrail
            pass

        if chunks_to_pop_out == 0:
            # didn't find any boundary indicator
            return 0

        # The order is important here as the webhook_modified_content will be passed into regex match
        # if we are to change the order in the future, need to make sure the regex_modifed_content is
        # used to pass into webhook. Cannot just swap the 2 sections.
        webhook_modified = False
        webhook_modified_contents: List[str] | None = None
        if webhook:
            with OtelTracer.get().start_as_current_span(
                "webhook",
                context=trace.set_span_in_context(parent_span),
            ):
                headers = deepcopy(resp_headers)
                TraceContextTextMapPropagator().inject(headers)
                (
                    webhook_modified,
                    webhook_modified_contents,
                ) = await call_response_webhook(
                    webhook_host=webhook.endpoint.host,
                    webhook_port=webhook.endpoint.port,
                    headers=headers,
                    contents=(content_data.content for content_data in contents),
                )
                if webhook_modified and webhook_modified_contents is not None:
                    if len(webhook_modified_contents) != len(contents):
                        logger.error(
                            f"guardrail response webhook response does not contains all choices of the original content {len(contents)} vs {len(webhook_modified_contents)} "
                        )
                        # set this to None so it won't get used
                        webhook_modified_contents = None
                    elif regex:
                        # we only need to do this if regex is also enabled; otherwise, webhook_modified_contents will be used directly
                        for i, item in enumerate(contents):
                            item.content = webhook_modified_contents[i]
        regex_modified = False
        regex_modified_contents: List[str] = []
        if regex:
            # regex_transform can throw RegexRejection exception. Deliberately not catching
            # it here so it bubbles up all the way to server.Process() so it can construct an
            # immediate error response there.
            with OtelTracer.get().start_as_current_span(
                "regex",
                context=trace.set_span_in_context(parent_span),
            ):
                for i, item in enumerate(contents):
                    logger.debug(f"regex: choice_index: {i} content: {item.content}")
                    regex_modified_contents.append(
                        regex_transform("", item.content, regex, anonymizer_engine)
                    )
                    if item.content != regex_modified_contents[i]:
                        # as long as there is one choice that got modified, we need to collapse
                        # the chunks for all choices so they are aligned
                        regex_modified = True
                        logger.debug(
                            f"regex: choice_index: {i} modifed_content: {regex_modified_contents[i]}"
                        )

        if regex_modified:
            # if webhook has modified the contents, the modified contents would have already pass into regex
            # so, only use the webhook_modified_contents if regex didn't modify them
            return self.collapse_chunks_with_new_content(
                llm_provider, contents, regex_modified_contents
            )
        elif webhook_modified and webhook_modified_contents is not None:
            return self.collapse_chunks_with_new_content(
                llm_provider, contents, webhook_modified_contents
            )

        # no modification, so pop out the chunks that already went through guardrails
        return chunks_to_pop_out

    def collect_stream_info(self, llm_provider: Provider, chunk: StreamChunkData):
        """
        This function get called on each chunk of the streaming response and try to
        collect information about the streaming response for use later
        """
        if not self.is_completed:
            # allow chunk.json_data to be None for calling this because on some API
            # the "completion" is indicated by a SSE data tag ["DONE"] without any json data
            self.is_completed = llm_provider.is_streaming_response_completed(chunk)

        if chunk.json_data is None:
            return

        if self.tokens is None:
            self.tokens = llm_provider.tokens(chunk.json_data)
        else:
            self.tokens += llm_provider.tokens(chunk.json_data)

        if self.model == "":
            self.model = llm_provider.get_model_resp(chunk.json_data)

        if not self.is_function_calling:
            self.is_function_calling = llm_provider.has_function_call_finish_reason(
                chunk.json_data
            )

        # TODO: get the role from the chunk

    async def buffer(
        self,
        llm_provider: Provider,
        resp_webhook: prompt_guard.Webhook | None,
        resp_regex: list[EntityRecognizer] | None,
        anonymizer_engine: AnonymizerEngine,
        resp_headers: dict[str, str],
        resp_body: external_processor_pb2.HttpBody,
        parent_span: trace.Span,
    ) -> bytes | None:
        """
        Buffer data for Guardrail. Returns the bytes when the data comes out of the Fifo
        """
        if resp_webhook is None and resp_regex is None:
            # Guardrail feature is not enabled, so no need to buffer
            return resp_body.body

        try:
            chunks, self.__leftover = sse.parse_sse_messages(
                llm_provider=llm_provider,
                data=resp_body.body,
                prev_leftover=self.__leftover,
            )
            for chunk in chunks:
                logger.debug("    StreamChunkData(")
                logger.debug(f"        raw_data = {chunk.raw_data}")
                logger.debug(f"        json_data = {chunk.json_data}")
                logger.debug(f"        type = {chunk.type.name}")
                logger.debug(f"        contents = {chunk.get_contents()}")
                # TODO(andy): if chunk type is BINARY, flush all the chunk and stop buffering until we see a text chunk again
                self.collect_stream_info(llm_provider=llm_provider, chunk=chunk)

                self.append_chunk(chunk)
        except Exception as exc:
            logger.error(f"error parsing_stream_chunks, {exc}")
            print(traceback.format_exc())
            return resp_body.body

        if resp_body.end_of_stream and self.__leftover:
            logger.critical(
                f"reached end of stream but still has leftover data: {self.__leftover}"
            )
            self.append_chunk(
                StreamChunkData(
                    raw_data=self.__leftover,
                    json_data=None,
                    contents=None,
                    type=StreamChunkDataType.INVALID,
                )
            )

        number_messages_to_remove = await self.do_guardrails_check(
            final=resp_body.end_of_stream,
            llm_provider=llm_provider,
            resp_headers=resp_headers,
            regex=resp_regex,
            webhook=resp_webhook,
            anonymizer_engine=anonymizer_engine,
            parent_span=parent_span,
        )

        return self.pop_chunks(number_messages_to_remove)

    #    __boundaryRegex = re.compile(r'([.,?!:;] +\n*|\n+)')
    __boundaryRegex = re.compile(
        r"([.?!;] +\n*|\n+)"
    )  # TODO(andy): should this be configurable

    def find_segment_boundary(
        self, contents: List[StreamChunksContent]
    ) -> List[BoundaryMatchData]:
        """
        Find a boundaryPattern specified by __boundaryRegex from the current contents in self.__contents that
        indicates a semantic segment boundary and return a BoundaryMatchData for the content that has a match.
        The BoundaryMatchData object only contains the last match (closest to the end)
        A boundary indicator is pre-compiled in __boundaryRegex class variable and is one of the
        punctuation . , , , ? , ! , : , ; followed by any white space or a newline by itself.

        Note about the newline character:
        A newline from the llm in the json field is converted to "\n" (2 characters across the wire) when the actual
        newline character is assigned to the field. When assigned the field value to a str in python or encode() into bytes,
        it's converted back to a single character. So, when matching, we need to match the single newline character
        and not 2 characters. This is probably true for any special ascii characters.
        """

        result: List[BoundaryMatchData] = []
        for i, item in enumerate(contents):
            for match in reversed(
                list(StreamChunks.__boundaryRegex.finditer(item.content))
            ):
                logger.debug(
                    f"found boundary: choice_index: {i} group: {match.group()} groups: {match.groups()} [{match.pos}, {match.endpos}) content: {item.content}"
                )
                result.append(
                    BoundaryMatchData(
                        choice_index=i,
                        capture=match.group(),
                        start_pos=match.start(),
                        end_pos=match.end(),
                    )
                )
                break

        return result

    def collapse_contents(
        self,
        llm_provider: Provider,
        choice_index: int,
        src_start_index: int,
        src_end_index: int,
        dst_index: int,
    ):
        """
        collapse_contents move content for one choice specified by choice_index from the src chunks pointed to
        from src_start_index (inclusive) to src_end_index (exclusive) into the dst chunk
        This function only get used from the logic where we assume the multi-choices response are in the array in
        a single chunk so we need to align the detected boundary in every choices to the same chunk. Turns out OpenAI
        doesn't do that and need to re-think the logic to support that.
        """
        logger.debug(
            f"collapse_contents: src_start: {src_start_index} src_end: {src_end_index} dst: {dst_index} fifo size: {len(self.__streaming_fifo)}"
        )
        dst_chunk = self.__streaming_fifo[dst_index]
        dst_chunk_contents = dst_chunk.get_contents()
        if dst_chunk_contents is None or dst_chunk.json_data is None:
            logger.critical(
                f"collapse_contents: dst chunk has no data! indexes: src_start: {src_start_index} src_end: {src_end_index} dst: {dst_index}"
            )
            return

        new_content = bytearray()
        # TODO(andy): bounds check all the indices
        prepend_to_dst = False
        if dst_index >= src_end_index:
            # prepend contents to dst chunk
            prepend_to_dst = True
        elif dst_index < src_start_index:
            # append contents to dst chunk
            new_content.extend(dst_chunk.get_content(choice_index))
        else:
            logger.critical(
                f"collapse_contents: invalid indexes: src_start: {src_start_index} src_end: {src_end_index} dst: {dst_index}"
            )
            return

        for i in range(src_start_index, src_end_index):
            chunk = self.__streaming_fifo[i]
            if chunk.get_contents() is None:
                continue
            existing_content = chunk.get_content(choice_index)
            if len(existing_content) == 0:
                continue

            new_content.extend(existing_content)

            # clear out the existing content
            if chunk.json_data is not None:
                update_chunk_content(
                    llm_provider, chunk, choice_index=choice_index, content=b""
                )

        if prepend_to_dst and dst_chunk.get_contents() is not None:
            new_content.extend(dst_chunk.get_content(choice_index))

        update_chunk_content(
            llm_provider,
            dst_chunk,
            choice_index=choice_index,
            content=bytes(new_content),
        )
