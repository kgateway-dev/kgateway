import json
import logging

from typing import Any, Dict, Final, List, Tuple
from ext_proc.streamchunkdata import StreamChunkData, StreamChunkDataType
from ext_proc.provider import Provider

logger = logging.getLogger().getChild("kgateway-ai-ext.util")

SSE_DELIMITER: Final[bytes] = b"\n\n"
SSE_DELIMITER_GEMINI: Final[bytes] = b"\r\n\r\n"
SSE_DATA_FIELD_NAME: Final[bytes] = b"data:"
SSE_DATA_DONE: Final[bytes] = b"[DONE]"


class SSEParsingException(Exception):
    """Server Sent Event Message Parsing Exception"""

    pass


def replace_json_data(raw_data: bytes, json_data: Dict[str, Any]) -> bytes:
    """
    This function works on a single sse message only. So, raw_data should only contains one `data:` field.
    It find where the json object within raw_data and replace it with the string dump of json_data.
    If there is no data field, it will just return raw_data as is.

    Throws SSEParsingException if no json object in data field
    """
    new_raw_data = bytearray()
    lines = raw_data.splitlines(keepends=True)
    for line in lines:
        if not line.startswith(SSE_DATA_FIELD_NAME):
            new_raw_data.extend(line)
            continue

        json_start_pos = line.find(b"{")
        json_end_pos = line.rfind(b"}") + 1

        logger.debug(f"json start: {json_start_pos} end: {json_end_pos}")

        if json_start_pos < 0 or json_end_pos < 0 or json_start_pos >= json_end_pos:
            raise SSEParsingException(
                f"failed to find json in SSE data message: {line}"
            )

        new_raw_data.extend(line[:json_start_pos])
        new_raw_data.extend(json.dumps(json_data).encode("utf-8"))
        if json_end_pos < len(line):
            new_raw_data.extend(line[json_end_pos:])

    return bytes(new_raw_data)


def parse_sse_messages(
    llm_provider: Provider, data: bytes, prev_leftover: bytes
) -> Tuple[List[StreamChunkData], bytes]:
    """
    parse_sse_messages parses the data from a http chunk and break them down into the corresponding
    raw data, json data (parsed from raw data) and the contents (from json data). For SSE, raw data is a
    single SSE Message. json data would be the parsed json inside the "data:" field. The contents would
    be the response messages from the LLM.

    Any incomplete data will be return as the leftover (bytes) which should be passed back into this function
    in the prev_leftover param.
    """
    chunks: List[StreamChunkData] = []

    sse_delimiter = llm_provider.get_sse_delimiter()
    sse_delimiter_len = len(sse_delimiter)

    # content response message from LLM can contain "\n" (2 characters), when it's stored into byte.
    # They don't get converted to a single newline char until we decode() it into string, so it should
    # not affect our parsing here even it might have "\n\n". I check the TCP dump that they are 4
    # characters in json over the wire.

    start_pos = 0

    empty_leftover = bytes()
    if len(prev_leftover) > 0:
        data = prev_leftover + data
    while start_pos < len(data):
        sse_end_pos = data.find(sse_delimiter, start_pos)
        if sse_end_pos < 0:
            logger.debug(
                f"cannot find SSE message delimiter! saving data to leftover: {data[start_pos:]}"
            )
            return chunks, data[start_pos:]

        if sse_end_pos < 5:
            # this means we have some newline before the actual SSE message, still include them in the raw_data
            # but skip them when parsing the message
            sse_end_pos += sse_delimiter_len
            if sse_end_pos > len(data):
                logger.critical("empty SSE message!")
                chunks.append(
                    StreamChunkData(
                        raw_data=data,
                        json_data=None,
                        contents=None,
                        type=StreamChunkDataType.INVALID,
                    )
                )
                return chunks, empty_leftover

            sse_end_pos = data.find(sse_delimiter, sse_end_pos)

        if sse_end_pos < 0:
            logger.debug(
                f"cannot find SSE message delimiter! saving data to leftover: {data[start_pos:]}"
            )
            return chunks, data[start_pos:]

        json_data = None
        lines = data[start_pos:sse_end_pos].splitlines()
        type: StreamChunkDataType = StreamChunkDataType.NORMAL_TEXT
        for line in lines:
            if line.startswith(SSE_DATA_FIELD_NAME):
                if line.endswith(SSE_DATA_DONE):
                    type = StreamChunkDataType.DONE
                    break
                try:
                    json_data = json.loads(
                        line[len(SSE_DATA_FIELD_NAME) :].decode("utf-8")
                    )

                except json.JSONDecodeError as e:
                    logger.error(
                        f"JSON decoding error occurred while parsing SSE message: {e} data:\n{line[len(SSE_DATA_FIELD_NAME) :]}"
                    )
                    type = StreamChunkDataType.INVALID
                    break  # Fall down to add the invalid chunk and then continue to parse the rest of the data
                else:
                    break

        sse_end_pos += sse_delimiter_len
        if sse_delimiter == SSE_DELIMITER_GEMINI:
            while (
                sse_end_pos + 1 < len(data)
                and data[sse_end_pos : sse_end_pos + 2] == b"\r\n"
            ):
                # Gemini uses '\r\n\r\n' instead of '\n\n'. In case there are extra '\r\n' between the data
                sse_end_pos += 2
        else:
            while sse_end_pos < len(data) and data[sse_end_pos] == b"\n":
                # The SSE spec says the delimiter should be 2 newline characters between messages but I have seen
                # openai randomly have more than 2 newlines between message, so account for that and include them
                # in the raw_data
                sse_end_pos += 1

        contents = None
        if json_data is not None:
            contents = llm_provider.extract_contents_from_resp_chunk(json_data)
            if type != StreamChunkDataType.DONE and type != StreamChunkDataType.INVALID:
                type = llm_provider.get_stream_resp_chunk_type(json_data)
        # if the chunk is "data: [DONE]" or we failed to parse the json,
        # json_data and contents will be None here
        chunks.append(
            StreamChunkData(
                raw_data=data[start_pos:sse_end_pos],
                json_data=json_data,
                contents=contents,
                type=type,
            )
        )

        start_pos = sse_end_pos

    return chunks, empty_leftover
