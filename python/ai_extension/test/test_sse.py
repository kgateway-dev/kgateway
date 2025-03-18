import json
import unittest

from util import sse
from ext_proc.streamchunkdata import StreamChunkDataType
from ext_proc.provider import Anthropic, Gemini, OpenAI


def openai_sse_data() -> bytes:
    return b'data: {"id":"chatcmpl-AoHzmSUIH7Edb8DxDAk0plcgzb6X9","object":"chat.completion.chunk","created":1736548938,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" they"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n'


def openai_multiple_sse_data() -> bytes:
    return b'data: {"id":"chatcmpl-AoHzmSUIH7Edb8DxDAk0plcgzb6X9","object":"chat.completion.chunk","created":1736548938,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" they"},"logprobs":null,"finish_reason":null}],"usage":null}\n\ndata: {"id":"chatcmpl-AoHzmSUIH7Edb8DxDAk0plcgzb6X9","object":"chat.completion.chunk","created":1736548938,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" may"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n'


def gemini_sse_data() -> bytes:
    return b'data: {"candidates": [{"content": {"parts": [{"text": "Here are some"}],"role": "model"},"index": 0,"safetyRatings": [{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HATE_SPEECH","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_HARASSMENT","probability": "NEGLIGIBLE"},{"category": "HARM_CATEGORY_DANGEROUS_CONTENT","probability": "NEGLIGIBLE"}]}],"usageMetadata": {"promptTokenCount": 21,"candidatesTokenCount": 3,"totalTokenCount": 24},"modelVersion": "gemini-1.5-flash-001"}\r\n\r\n'


def gemini_multiple_sse_data() -> bytes:
    return b'data: {"candidates": [{"content": {"role": "model","parts": [{"text": "\\n* **Don\'t use real email addresses:** Always use placeholder email addresses when testing to avoid accidentally sending emails to real people.\\n* **Use appropriate domains:** Choose domain names that clearly indicate the purpose of the email address, such as"}]},"safetyRatings": [{"category": "HARM_CATEGORY_HATE_SPEECH","probability": "NEGLIGIBLE","probabilityScore": 0.06298828,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.10498047},{"category": "HARM_CATEGORY_DANGEROUS_CONTENT","probability": "NEGLIGIBLE","probabilityScore": 0.17480469,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.099609375},{"category": "HARM_CATEGORY_HARASSMENT","probability": "NEGLIGIBLE","probabilityScore": 0.1484375,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.03955078},{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT","probability": "NEGLIGIBLE","probabilityScore": 0.056640625,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.048095703}]}],"modelVersion": "gemini-1.5-flash-001"}\r\n\r\ndata: {"candidates": [{"content": {"role": "model","parts": [{"text": " \\"test.com\\" or \\"sample.net.\\"\\n\\nPlease note: While these examples are safe to use for testing purposes, you should never use real personal information in your testing. \\n"}]},"finishReason": "STOP","safetyRatings": [{"category": "HARM_CATEGORY_HATE_SPEECH","probability": "NEGLIGIBLE","probabilityScore": 0.06933594,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.11425781},{"category": "HARM_CATEGORY_DANGEROUS_CONTENT","probability": "NEGLIGIBLE","probabilityScore": 0.25195313,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.10839844},{"category": "HARM_CATEGORY_HARASSMENT","probability": "NEGLIGIBLE","probabilityScore": 0.12890625,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.03515625},{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT","probability": "NEGLIGIBLE","probabilityScore": 0.08886719,"severity": "HARM_SEVERITY_NEGLIGIBLE","severityScore": 0.06298828}]}],"usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 239,"totalTokenCount": 259},"modelVersion": "gemini-1.5-flash-001"}\r\n\r\n'


def anthropic_sse_data() -> bytes:
    return b'event: content_block_delta\ndata: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}\n\n'


class SSETestCase(unittest.TestCase):
    def test_replace_json_data(self):
        # nothing should change on non-sse data
        data = b"junk\nmore junk\n\n"
        jsn = {}
        output = sse.replace_json_data(data, jsn)
        assert output == data

        # sample openai sse data
        jsn = json.loads('{"foo": "bar", "test": 123}')
        output = sse.replace_json_data(openai_sse_data(), jsn)
        assert output == b'data: {"foo": "bar", "test": 123}\n\n'

        # sample gemini sse data
        output = sse.replace_json_data(gemini_sse_data(), jsn)
        assert output == b'data: {"foo": "bar", "test": 123}\r\n\r\n'

        # sample anthropic sse data
        output = sse.replace_json_data(anthropic_sse_data(), jsn)
        assert (
            output
            == b'event: content_block_delta\ndata: {"foo": "bar", "test": 123}\n\n'
        )

        # no json in data field
        data = b"data: junk\n\n"
        with self.assertRaises(expected_exception=sse.SSEParsingException) as context:
            sse.replace_json_data(data, jsn)
        self.assertTrue(len(str(context.exception)) > 0)

    def test_parse_sse_messages(self):
        # completed OpenAI SSE data with 2 chunks
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=OpenAI(), data=openai_multiple_sse_data(), prev_leftover=b""
        )
        assert len(chunks) == 2
        assert chunks[0].type == StreamChunkDataType.NORMAL_TEXT
        assert chunks[1].type == StreamChunkDataType.NORMAL_TEXT
        assert leftover == b""

        # completed Gemini SSE data with 2 chunks
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=Gemini(), data=gemini_multiple_sse_data(), prev_leftover=b""
        )
        assert len(chunks) == 2
        assert chunks[0].type == StreamChunkDataType.NORMAL_TEXT
        assert chunks[1].type == StreamChunkDataType.FINISH
        assert leftover == b""

        # completed Anthropic SSE data with 1 chunk
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=Anthropic(), data=anthropic_sse_data(), prev_leftover=b""
        )
        assert len(chunks) == 1
        assert chunks[0].type == StreamChunkDataType.NORMAL_TEXT
        assert leftover == b""

        # incomplete OpenAI SSE data with less than 1 chunk
        data = openai_sse_data()
        part1 = data[0 : len(data) - 20]
        part2 = data[len(data) - 20 :]
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=OpenAI(), data=part1, prev_leftover=b""
        )
        assert len(chunks) == 0
        assert leftover == part1
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=OpenAI(), data=part2, prev_leftover=leftover
        )
        assert len(chunks) == 1
        assert chunks[0].type == StreamChunkDataType.NORMAL_TEXT
        assert leftover == b""

        # incomplete Gemini SSE data with 1 complete chunk and 1 partial chunk
        data = gemini_multiple_sse_data()
        part1 = data[0 : len(data) - 20]
        part2 = data[len(data) - 20 :]
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=Gemini(), data=part1, prev_leftover=b""
        )
        assert len(chunks) == 1
        assert chunks[0].type == StreamChunkDataType.NORMAL_TEXT
        assert leftover.startswith(b"data: ")
        chunks, leftover = sse.parse_sse_messages(
            llm_provider=Gemini(), data=part2, prev_leftover=leftover
        )
        assert len(chunks) == 1
        assert chunks[0].type == StreamChunkDataType.FINISH
        assert leftover == b""
