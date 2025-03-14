import json
import unittest

from ext_proc.streamchunks import (
    find_max_min_end_index,
    StreamChunks,
    StreamChunksContent,
    split_chunk_content_by_boundary_indicator,
)
from ext_proc.streamchunkdata import StreamChunkData, StreamChunkDataType
from typing import List
from ext_proc.provider import OpenAI, Provider
from test.test_data.sample_chunk_data import (
    test_chunk_data,
    utf8_test_chunk_data,
    multichoices_test_chunk_data,
)
from copy import deepcopy
from util.sse import parse_sse_messages


class TestStreamChunks(unittest.TestCase):
    def test_find_segment_boundary(self):
        chunks = StreamChunks()
        # The content from chunk 0 to 26 should be:
        #     In the heart of the code, a dance unfolds,
        #     Where whispers of logic, in layers, are told.
        #     A
        for chunk in test_chunk_data[0:27]:
            chunks.append_chunk(deepcopy(chunk))

        contents = chunks.get_contents_with_chunk_indices()
        matchData = chunks.find_segment_boundary(contents)
        assert matchData is not None
        assert len(matchData) == 1
        assert matchData[0].capture == ".  \n"
        assert matchData[0].start_pos == 89, (
            f"contents: {contents} len: {len(contents[0].content)}"
        )
        assert matchData[0].end_pos == 93, (
            f"contents: {contents} len: {len(contents[0].content)}"
        )

    def test_collapse_chunks_with_new_content(self):
        chunks = StreamChunks()
        # The content from chunk 0 to 10 should be:
        #     'In the heart of the code. a dance' (without quote)
        for chunk in test_chunk_data[0:10]:
            chunks.append_chunk(deepcopy(chunk))

        contents = chunks.get_contents_with_chunk_indices()
        assert contents[0].begin_index == 0
        assert contents[0].end_index == 10
        assert contents[0].content == "In the heart of the code. a dance"

        # The content from chunk 10 to 20 should be:
        #     ' unfolds,
        #      Where whispers of logic, in layers' (without quote)
        # Because we already grab the content and indices above,
        # these won't get collapsed, just appending it here to make sure
        # these are unchanged after the collapse_chunks_with_new_content() call
        for chunk in test_chunk_data[10:20]:
            chunks.append_chunk(deepcopy(chunk))

        new_contents: List[str] = []
        test_str = "this is a test"
        new_contents.append(test_str)
        provider = OpenAI()
        chunks_to_pop = chunks.collapse_chunks_with_new_content(
            llm_provider=provider, original_contents=contents, new_contents=new_contents
        )
        assert chunks_to_pop == 2

        contents = chunks.get_contents_with_chunk_indices()

        assert contents[0].begin_index == 0
        assert contents[0].end_index == 12
        assert (
            contents[0].content
            == test_str + " unfolds,  \nWhere whispers of logic, in layers"
        )

        # first chunk is an empty content chunk, so it's skipped from collapsing
        self.check_chunk_content(provider, chunks.pop_chunk(), 1, 0, b"")

        self.check_chunk_content(
            provider, chunks.pop_chunk(), 1, 0, test_str.encode("utf-8")
        )

        contents = chunks.get_contents_with_chunk_indices()

        assert contents[0].begin_index == 0
        assert contents[0].end_index == 10
        assert contents[0].content == " unfolds,  \nWhere whispers of logic, in layers"

    def test_collapse_chunks_with_new_content_multichoices(self):
        chunks = StreamChunks()
        # The content from chunk 0 to 25 should be:
        # choice 0: Okay! Imagine you have a big box
        # choice 1: Okay! Imagine you have a big,
        # choice 2: Okay! Imagine LLM is like
        for chunk in multichoices_test_chunk_data[0:26]:
            chunks.append_chunk(deepcopy(chunk))

        contents = chunks.get_contents_with_chunk_indices()

        # Because we already grab the content and indices above,
        # these won't get collapsed, just appending it here to make sure
        # these are unchanged after the collapse_chunks_with_new_content() call
        for chunk in multichoices_test_chunk_data[26:30]:
            chunks.append_chunk(deepcopy(chunk))

        assert len(contents) == 3, f"contents: {contents}"

        for content in contents:
            assert content.begin_index == 0
            assert content.end_index == 26
        assert contents[0].content == "Okay! Imagine you have a big box"
        assert contents[1].content == "Okay! Imagine you have a big,"
        assert contents[2].content == "Okay! Imagine LLM is like"

        new_contents: List[str] = []
        new_contents.append(contents[0].content.replace("you", "nobody"))
        new_contents.append(contents[1].content.replace("big", "small"))
        new_contents.append(contents[2].content.replace("LLM", "SLM"))
        provider = OpenAI()
        chunks_to_pop = chunks.collapse_chunks_with_new_content(
            llm_provider=provider, original_contents=contents, new_contents=new_contents
        )

        assert chunks_to_pop == 6, f"{chunks.pop_chunks(chunks_to_pop)}"

        contents = chunks.get_contents_with_chunk_indices()

        # There is an empty chunk at the beginning
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 0, b"")
        self.check_chunk_content(
            provider, chunks.pop_chunk(), 3, 0, b"Okay! Imagine nobody have a big box"
        )

        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 1, b"")
        self.check_chunk_content(
            provider, chunks.pop_chunk(), 3, 1, b"Okay! Imagine you have a small,"
        )

        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 2, b"")
        self.check_chunk_content(
            provider, chunks.pop_chunk(), 3, 2, b"Okay! Imagine SLM is like"
        )

        contents = chunks.get_contents_with_chunk_indices()

        for content in contents:
            assert content.begin_index == 0
            assert content.end_index == 4
        assert contents[0].content == " of"
        assert contents[1].content == " smart"
        assert contents[2].content == " a really"

    def test_align_contents_for_guardrail(self):
        chunks = StreamChunks()
        # The content from chunk 0 to 27 should be:
        #     In the heart of the code, a dance unfolds,
        #     Where whispers of logic, in layers, are told.
        #     A mystery
        # Testing the boundary indicator span 2 chunks (chunk #23 and #24)
        # but 2nd chunk ends with the indicator
        for chunk in test_chunk_data[0:27]:
            chunks.append_chunk(deepcopy(chunk))

        provider = OpenAI()
        contents = chunks.get_contents_with_chunk_indices()
        # TODO(andy): test multi-choice response
        all_chunks_contents_before_alignment = contents[0].content
        boundary_indicator = ".  \n"
        assert not contents[0].content.endswith(boundary_indicator)
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=20
        )
        assert contents[0].content.endswith(boundary_indicator)
        assert chunks_to_pop == 25
        contents = chunks.get_contents_with_chunk_indices()
        assert contents[0].content == all_chunks_contents_before_alignment

        chunks.pop_chunks(chunks_to_pop)
        contents = chunks.get_contents_with_chunk_indices()

        # Testing only double newline in a single chunk (chunk #34)
        chunks.pop_all()
        for chunk in test_chunk_data[0:37]:
            chunks.append_chunk(deepcopy(chunk))
        contents = chunks.get_contents_with_chunk_indices()
        all_chunks_contents_before_alignment = contents[0].content
        boundary_indicator = "\n\n"
        assert not contents[0].content.endswith(boundary_indicator)
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=20
        )
        assert contents[0].content.endswith(boundary_indicator)
        contents = chunks.get_contents_with_chunk_indices()
        assert contents[0].content == all_chunks_contents_before_alignment
        assert chunks_to_pop == 35

        # Testing the boundary indicator span 2 chunks (chunk #7 and #8)
        # but extra data in 2nd chunk
        chunks.pop_all()
        for chunk in test_chunk_data[0:10]:
            chunks.append_chunk(deepcopy(chunk))
        contents = chunks.get_contents_with_chunk_indices()
        all_chunks_contents_before_alignment = contents[0].content
        boundary_indicator = ". "
        assert not contents[0].content.endswith(boundary_indicator)
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=20
        )
        assert contents[0].content.endswith(boundary_indicator)
        contents = chunks.get_contents_with_chunk_indices()
        assert contents[0].content == all_chunks_contents_before_alignment
        assert chunks_to_pop == 8

        # TODO(andy): test boundary pos less than min_content_length case

        # TODO(andy): test aligning in the last chunk

    def check_chunk_content(
        self,
        llm_provider: Provider,
        chunk: StreamChunkData | None,
        total_choices: int,
        choice_index: int,
        expected_content: bytes,
    ):
        # The raw_data in chunk is ultimately what get sent to the end user. All other fields are for
        # efficiency and convenience. When testing, it's important to make sure raw_data is correct as
        # other fields might be out of sync due to coding error. So, this function re-parse the raw_data
        # to create a new chunk and do all the validation there.
        assert chunk is not None
        chunks, leftover = parse_sse_messages(llm_provider, chunk.raw_data, b"")

        # in raw_data, there should only be 1 single complete SSE message
        assert len(leftover) == 0
        assert len(chunks) == 1

        for i in range(0, total_choices):
            if i == choice_index:
                assert chunks[0].get_content(i) == expected_content, f"{chunks[0]}"
            else:
                assert chunks[0].get_content(i) == b"", f"{chunks[0]}"

    def test_align_contents_for_guardrail_multichoices(self):
        chunks = StreamChunks()
        provider = OpenAI()
        for chunk in multichoices_test_chunk_data[0:65]:
            chunks.append_chunk(deepcopy(chunk))

        contents = chunks.get_contents_with_chunk_indices()
        assert len(contents) == 3
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=50
        )

        # There are 3 choices from chunk #0 to #64, only 2 of the choices (0 and 2)
        # have the boundary indicator. So, chunks_to_pop is 0
        assert chunks_to_pop == 0

        for chunk in multichoices_test_chunk_data[65:68]:
            chunks.append_chunk(deepcopy(chunk))
        contents = chunks.get_contents_with_chunk_indices()
        assert len(contents) == 3
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=50
        )
        # chunk #67 has the boundary indicator for choice 2, so now the contents
        # will be algined before sending to guardrail
        # chunk# that has end of the boundary indicator for each choice:
        # choice 0: #60
        # choice 1: #67
        # choide 2: #62

        # Look at the table in step 3 of the slab doc link above will make this
        # much easier to understand.
        assert chunks_to_pop == 60
        chunks.pop_chunks(57)
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 0, b". ")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 1, b" new things. ")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 2, b". ")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 0, b"When")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 1, b"")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 2, b"This")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 0, b" you")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 1, b"")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 2, b" robot")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 0, b" want")
        self.check_chunk_content(provider, chunks.pop_chunk(), 3, 1, b"This")
        next_chunk = chunks.pop_chunk()
        assert next_chunk is None

    def test_align_contents_for_guardrail_utf8(self):
        chunks = StreamChunks()
        provider = OpenAI()
        for i, chunk in enumerate(utf8_test_chunk_data[0:34]):
            chunks.append_chunk(deepcopy(chunk))

        contents = chunks.get_contents_with_chunk_indices()
        assert len(contents) == 1
        chunks_to_pop = chunks.align_contents_for_guardrail(
            llm_provider=provider, stream_contents=contents, min_content_length=50
        )
        assert chunks_to_pop == 32

        chunks.pop_chunks(chunks_to_pop - 1)
        # This is chunk #31 originally has contents=[b"!  "],
        # The space from the next chunk should moved up to here, so should be b"!     " now
        self.check_chunk_content(provider, chunks.pop_chunk(), 1, 0, b"!     ")

        # This is chunk #32 originally has contents=[b"   \xf0\x9f\x96\xb1"],
        # the space should have moved up to chunk #31, so should be b"\xf0\x9f\x96\xb1" now
        self.check_chunk_content(
            provider, chunks.pop_chunk(), 1, 0, b"\xf0\x9f\x96\xb1"
        )

        chunks.pop_all()

    def test_adjust_contents_size(self):
        chunks = StreamChunks()
        assert len(chunks.get_contents_with_chunk_indices()) == 0
        dummy_contents = [b"", b""]
        new_chunk = StreamChunkData(
            b"junk", json.loads("{}"), dummy_contents, StreamChunkDataType.NORMAL_TEXT
        )
        chunks.adjust_contents_size(new_chunk)
        assert len(chunks.get_contents_with_chunk_indices()) == len(dummy_contents)

        dummy_contents.append(b"")
        chunks.adjust_contents_size(new_chunk)
        assert len(chunks.get_contents_with_chunk_indices()) == len(dummy_contents)

    def test_set_role(self):
        chunks = StreamChunks()
        chunks.set_role(1, "user")
        assert chunks.get_role(0) == ""
        assert chunks.get_role(1) == "user"

    def test_delete_chunks(self):
        chunks = StreamChunks()
        for i, chunk in enumerate(utf8_test_chunk_data[0:10]):
            chunks.append_chunk(deepcopy(chunk))

        # delete at the end (chunk 7, 8, 9), so should have 7 chunks left
        chunks.delete_chunks(7, 10)
        for i in range(7):
            assert utf8_test_chunk_data[i] == chunks.pop_chunk()
        # no more chunks after poping the remaining chunks
        assert chunks.pop_chunk() is None

        for i, chunk in enumerate(utf8_test_chunk_data[0:10]):
            chunks.append_chunk(deepcopy(chunk))

        # delete at the beginning (chunk 0, 1, 2, 3), so should have 6 chunks left
        chunks.delete_chunks(0, 4)
        for i in range(6):
            assert utf8_test_chunk_data[i + 4] == chunks.pop_chunk()
        # no more chunks after poping the remaining chunks
        assert chunks.pop_chunk() is None

        for i, chunk in enumerate(utf8_test_chunk_data[0:10]):
            chunks.append_chunk(deepcopy(chunk))
        # delete at the middle (chunk 4, 5), so should have 4 + 4 chunks left
        chunks.delete_chunks(4, 6)

        for i in range(4):
            assert utf8_test_chunk_data[i] == chunks.pop_chunk()
        for i in range(4):
            assert utf8_test_chunk_data[i + 6] == chunks.pop_chunk()
        # no more chunks after poping the remaining chunks
        assert chunks.pop_chunk() is None

    def createTestChunk(
        self, llm_provider: Provider, choice_index: int, content: bytes
    ) -> StreamChunkData:
        raw_data = b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":__INDEX__,"delta":{"content":"__CONTENT__"},"logprobs":null,"finish_reason":null}]}\n\n'
        raw_data = raw_data.replace(b"__INDEX__", str(choice_index).encode("utf-8"))
        raw_data = raw_data.replace(b"__CONTENT__", content)

        chunks, _ = parse_sse_messages(llm_provider, raw_data, b"")
        return chunks[0]

    def test_split_chunk_content_by_boundary_indicator(self):
        provider = OpenAI()
        chunk = self.createTestChunk(provider, 0, b".  This is ")
        boundary_indicator = b".  "
        bytes_stripped = split_chunk_content_by_boundary_indicator(
            provider, chunk, boundary_indicator, 0
        )
        self.check_chunk_content(provider, chunk, 1, 0, b"This is ")
        assert bytes_stripped == boundary_indicator

        chunk = self.createTestChunk(provider, 0, b"foo bar.  This is ")
        boundary_indicator = b".  "
        bytes_stripped = split_chunk_content_by_boundary_indicator(
            provider, chunk, boundary_indicator, 0
        )
        self.check_chunk_content(provider, chunk, 1, 0, b"This is ")
        assert bytes_stripped == b"foo bar.  "

        chunk = self.createTestChunk(provider, 0, b"  This is ")
        boundary_indicator = b".  "
        bytes_stripped = split_chunk_content_by_boundary_indicator(
            provider, chunk, boundary_indicator, 0
        )
        self.check_chunk_content(provider, chunk, 1, 0, b"This is ")
        assert bytes_stripped == b"  "

    def test_find_max_min_end_index(self):
        stream_contents = [
            StreamChunksContent(content="abc", begin_index=0, end_index=5),
            StreamChunksContent(content="123", begin_index=0, end_index=2),
            StreamChunksContent(content="foobar", begin_index=0, end_index=7),
        ]

        high, low = find_max_min_end_index(stream_contents)
        assert high == 7 and low == 2

    def createTestMultiChoiceStreamChunks(self, provider: Provider) -> StreamChunks:
        chunks = StreamChunks()
        chunks.append_chunk(self.createTestChunk(provider, 0, b""))  # 0
        chunks.append_chunk(self.createTestChunk(provider, 0, b"This is "))  # 1
        chunks.append_chunk(self.createTestChunk(provider, 1, b""))  # 2
        chunks.append_chunk(self.createTestChunk(provider, 1, b"This is"))  # 3
        chunks.append_chunk(self.createTestChunk(provider, 2, b""))  # 4
        chunks.append_chunk(self.createTestChunk(provider, 2, b"This is"))  # 5
        chunks.append_chunk(
            self.createTestChunk(provider, 0, b"a test. ")
        )  # 6 choice 0 boundary indicator
        chunks.append_chunk(self.createTestChunk(provider, 1, b"not "))  # 7
        chunks.append_chunk(self.createTestChunk(provider, 2, b"what "))  # 8
        chunks.append_chunk(self.createTestChunk(provider, 0, b"Nothing "))  # 9
        chunks.append_chunk(
            self.createTestChunk(provider, 1, b"a test. ")
        )  # 10 choice 1 boundary indicator
        chunks.append_chunk(self.createTestChunk(provider, 2, b"a test "))  # 11
        chunks.append_chunk(self.createTestChunk(provider, 0, b"to "))  # 12
        chunks.append_chunk(self.createTestChunk(provider, 1, b"Nothing "))  # 13
        chunks.append_chunk(
            self.createTestChunk(provider, 2, b"should be. ")
        )  # 14 choice 2 boundary indicator
        chunks.append_chunk(self.createTestChunk(provider, 0, b"see "))  # 15
        chunks.append_chunk(self.createTestChunk(provider, 1, b"to "))  # 16
        chunks.append_chunk(self.createTestChunk(provider, 2, b"Nothing "))  # 17

        return chunks

    def test_find_chunk_with_boundary_indicator(self):
        provider = OpenAI()
        # boundary indicator split across 2 chunks
        chunks = StreamChunks()
        chunks.append_chunk(self.createTestChunk(provider, 0, b"This is "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"not a test."))
        chunks.append_chunk(self.createTestChunk(provider, 0, b" Nothing to see"))

        stream_contents = chunks.get_contents_with_chunk_indices()
        assert len(stream_contents) == 1
        match_list = chunks.find_segment_boundary(stream_contents)
        assert len(match_list) == 1

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[0], stream_contents[0]
        )
        assert chunk_index == 2

        # boundary indicator split across 3 chunks
        chunks = StreamChunks()
        chunks.append_chunk(self.createTestChunk(provider, 0, b"This is "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"not a test."))
        chunks.append_chunk(self.createTestChunk(provider, 0, b" "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b" Nothing to see"))

        stream_contents = chunks.get_contents_with_chunk_indices()
        assert len(stream_contents) == 1
        match_list = chunks.find_segment_boundary(stream_contents)
        assert len(match_list) == 1

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[0], stream_contents[0]
        )
        assert chunk_index == 3

        # boundary indicator at the end of a chunk
        chunks = StreamChunks()
        chunks.append_chunk(self.createTestChunk(provider, 0, b"This is "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"not a test.  \\n"))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"Nothing to see here."))

        stream_contents = chunks.get_contents_with_chunk_indices()
        assert len(stream_contents) == 1
        match_list = chunks.find_segment_boundary(stream_contents)
        assert len(match_list) == 1

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[0], stream_contents[0]
        )
        assert chunk_index == 1

        # boundary indicator at the middle of a chunk
        chunks = StreamChunks()
        chunks.append_chunk(self.createTestChunk(provider, 0, b"This is "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"not a test.  Nothing "))
        chunks.append_chunk(self.createTestChunk(provider, 0, b"to see here."))

        stream_contents = chunks.get_contents_with_chunk_indices()
        assert len(stream_contents) == 1
        match_list = chunks.find_segment_boundary(stream_contents)
        assert len(match_list) == 1

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[0], stream_contents[0]
        )
        assert chunk_index == 1

        # multi-choices
        chunks = self.createTestMultiChoiceStreamChunks(provider)
        stream_contents = chunks.get_contents_with_chunk_indices()
        assert len(stream_contents) == 3
        match_list = chunks.find_segment_boundary(stream_contents)
        assert len(match_list) == 3

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[0], stream_contents[0]
        )
        assert chunk_index == 6

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[1], stream_contents[1]
        )
        assert chunk_index == 10

        chunk_index = chunks.find_chunk_with_boundary_indicator(
            match_list[2], stream_contents[2]
        )
        assert chunk_index == 14

    def test_find_prev_chunk_containing_choice(self):
        provider = OpenAI()
        chunks = self.createTestMultiChoiceStreamChunks(provider)
        prev_index, prev_chunk = chunks.find_prev_chunk_containing_choice(
            provider, current_chunk_index=13, choice_index=2
        )
        assert prev_index == 11
        assert len(prev_chunk.get_content(0)) == 0
        assert len(prev_chunk.get_content(1)) == 0
        assert len(prev_chunk.get_content(2)) > 0

        prev_index, prev_chunk = chunks.find_prev_chunk_containing_choice(
            provider, current_chunk_index=13, choice_index=0
        )
        assert prev_index == 12
        assert len(prev_chunk.get_content(0)) > 0
        assert len(prev_chunk.get_content(1)) == 0
        assert len(prev_chunk.get_content(2)) == 0

        prev_index, prev_chunk = chunks.find_prev_chunk_containing_choice(
            provider, current_chunk_index=13, choice_index=1
        )
        assert prev_index == 10
        assert len(prev_chunk.get_content(0)) == 0
        assert len(prev_chunk.get_content(1)) > 0
        assert len(prev_chunk.get_content(2)) == 0

    def test_get_usage_from_chunks(self):
        chunks = StreamChunks()
        for chunk in multichoices_test_chunk_data[400:]:
            chunks.append_chunk(deepcopy(chunk))

        provider = OpenAI()
        usages = chunks.get_usage_from_chunks(
            provider.tokens, 0, len(multichoices_test_chunk_data) - 400
        )
        assert usages.prompt == 23
        assert usages.completion == 408
