import logging

from typing import List
from typing import Dict
from typing import Any
from enum import Enum

logger = logging.getLogger().getChild("kgateway-ai-ext.streamchunkdata")


class StreamChunkDataType(Enum):
    NORMAL_TEXT = 1  # a normal chunk with content
    NORMAL_BINARY = (
        2  # a chunk contains binary data like audio and have not text content
    )
    FINISH = 3  # a chunk with finish_reason that's not null.
    FINISH_NO_CONTENT = (
        4  # a chunk with finish_reason that's not null and no text content.
    )
    # For openai, this chunk normal does not have content and it's before the DONE chunk
    DONE = 5  # the chunk with `data: [DONE]` and has no json data
    INVALID = 6  # chunk that contains invalid json or empty SSE message
    LAST_USAGE = (
        7  # the last chunk the contains the total usage but no contents (Open AI)
    )
    UNKNOWN = 8


class StreamChunkData:
    __slot__ = ("raw_data", "json_data", "contents")

    def __init__(self, raw_data, json_data, contents, type):
        self.raw_data: bytes = raw_data
        self.json_data: Dict[str, Any] | None = json_data
        self.__contents: List[bytes] | None = contents
        self.type: StreamChunkDataType = type
        # TODO(andy): there doesn't seems to be much value to provide role to the webhook in the response
        #             because there is only one role in response. Some API does not have a role in the response
        #             at all
        # self.roles: List[bytes] | None = roles
        # TODO(andy): add usage (tokens) data here as well?

    def __repr__(self) -> str:
        return f"raw_data:\n{self.raw_data}\njson_data:\n{self.json_data}\ncontents:\n{self.__contents}"

    def __eq__(self, other):
        if not isinstance(other, StreamChunkData):
            # don't attempt to compare against unrelated types
            return NotImplemented

        return (
            self.raw_data == other.raw_data
            and self.json_data == other.json_data
            and self.__contents == other.__contents
            and self.type == other.type
        )

    def get_content_length(self, choice_index: int) -> int:
        if self.__contents is None or len(self.__contents) <= choice_index:
            return 0

        content: bytes = self.__contents[choice_index]
        # The bytes can contain non-ascii character, mostly likely utf-8 encoded.
        # so need to return the decoded length instead
        # Ashish did some perf test, decode() is very fast and checking
        # if the bytes list has any non-ascii using `any` is very slow
        # so, just calling decode() here
        return len(content.decode("utf-8"))

    def get_content(self, choice_index: int) -> bytes:
        """
        get_content() returns the bytes for the specified choice_index. Because it's possible
        for a chunk to have less than the number of choices, it will return
        empty bytes if the index is out of bounds
        """
        if self.__contents is None:
            return b""

        if choice_index < 0 or choice_index >= len(self.__contents):
            return b""

        return self.__contents[choice_index]

    def get_contents(self) -> List[bytes] | None:
        return self.__contents

    def set_contents(self, new_contents: List[bytes] | None):
        self.__contents = new_contents

    def set_content(self, choice_index: int, content: bytes):
        if self.__contents is None:
            return

        if choice_index >= len(self.__contents):
            logger.warning(
                "set_content() choice_index %d out of bound. size of contents list: %d",
                choice_index,
                len(self.__contents),
            )
            return
        self.__contents[choice_index] = content

    def adjust_contents_size(self, choice_index: int):
        """
        adjust the size of the self.__contents list as needed to be able to
        hold the content with choice_index
        """
        if self.__contents is None:
            return

        contents_size = len(self.__contents)
        self.__contents.extend([b""] * (choice_index + 1 - contents_size))
