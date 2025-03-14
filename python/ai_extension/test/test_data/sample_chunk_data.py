import json

from ext_proc.streamchunkdata import StreamChunkData, StreamChunkDataType
from typing import List

multichoices_test_chunk_data: List[StreamChunkData] = [
    # This chunk data represent the multi-choice response chunks of the following response:
    # choice 0:
    # Okay! Imagine you have a big box of crayons, and each crayon has different colors. When you want to draw something, you pick the crayons you need to make your picture.
    #
    # Now, think of LLM, which stands for "Large Language Model," like a super smart robot that knows a lot about words and stories. It has read a huge number of books, just like how you learn to draw by practicing with your crayons.
    #
    # When you ask the robot a question or tell it to make up a story, it looks in its big memory (like your box of crayons) and picks the right words to help you. So, it can talk to you, answer your questions, or tell you fun stories, just like you use your crayons to make colorful pictures!
    # choice 1:
    # Okay! Imagine you have a big, smart robot friend who loves to read stories and learn new things. This robot is called an LLM, which stands for "Large Language Model."
    #
    # Just like you learn words and how to put them together to make sentences, the LLM has read lots and lots of books, articles, and stories. So, it knows a lot about how to talk and write!
    #
    # When you ask the robot a question or tell it to make a story, it thinks really fast and uses all the things it learned to give you an answer or tell you a fun tale. It doesn’t know everything, but it tries its best to help you with whatever you ask! It’s like having a super smart, talking book that can chat with you!
    # choice 2:
    # Okay! Imagine LLM is like a really smart robot friend that loves to read and talk. This robot has read a lot of books, stories, and all kinds of words, so it knows a lot about many things.
    #
    # When you ask it a question or talk to it, it uses all the things it has learned to give you answers or tell you stories. It’s like having a super helper who is always ready to chat and share what it knows!
    # 0
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"role": "assistant", "content": "", "refusal": null}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b""],
    ),
    # 1
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"Okay"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "Okay"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Okay"],
    ),
    # 2
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"role": "assistant", "content": "", "refusal": null}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b""],
    ),
    # 3
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"Okay"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "Okay"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"Okay"],
    ),
    # 4
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"role": "assistant", "content": "", "refusal": null}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b""],
    ),
    # 5
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"Okay"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "Okay"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"Okay"],
    ),
    # 6
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"!"],
    ),
    # 7
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"!"],
    ),
    # 8
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"!"],
    ),
    # 9
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" Imagine"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " Imagine"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" Imagine"],
    ),
    # 10
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" Imagine"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " Imagine"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" Imagine"],
    ),
    # 11
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" Imagine"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " Imagine"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" Imagine"],
    ),
    # 12
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 13
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 14
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" L"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " L"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" L"],
    ),
    # 15
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" have"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " have"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" have"],
    ),
    # 16
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" have"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " have"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" have"],
    ),
    # 17
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"LM"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "LM"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"LM"],
    ),
    # 18
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 19
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 20
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " is"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" is"],
    ),
    # 21
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" big"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " big"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" big"],
    ),
    # 22
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" big"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " big"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" big"],
    ),
    # 23
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" like"],
    ),
    # 24
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" box"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " box"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" box"],
    ),
    # 25
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 26
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" a"],
    ),
    # 27
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" of"],
    ),
    # 28
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" smart"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " smart"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" smart"],
    ),
    # 29
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" really"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " really"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" really"],
    ),
    # 30
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" crayons"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " crayons"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" crayons"],
    ),
    # 31
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" robot"],
    ),
    # 32
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" smart"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " smart"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" smart"],
    ),
    # 33
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 34
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" friend"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " friend"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" friend"],
    ),
    # 35
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" robot"],
    ),
    # 36
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" and"],
    ),
    # 37
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" who"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " who"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" who"],
    ),
    # 38
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" friend"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " friend"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" friend"],
    ),
    # 39
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" each"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " each"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" each"],
    ),
    # 40
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" loves"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " loves"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" loves"],
    ),
    # 41
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" that"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " that"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" that"],
    ),
    # 42
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" cr"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " cr"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" cr"],
    ),
    # 43
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 44
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" loves"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " loves"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" loves"],
    ),
    # 45
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"ayon"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "ayon"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"ayon"],
    ),
    # 46
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" read"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " read"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" read"],
    ),
    # 47
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" to"],
    ),
    # 48
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" has"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " has"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" has"],
    ),
    # 49
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" stories"],
    ),
    # 50
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" read"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " read"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" read"],
    ),
    # 51
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" different"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " different"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" different"],
    ),
    # 52
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 53
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" and"],
    ),
    # 54
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" colors"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " colors"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" colors"],
    ),
    # 55
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" learn"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " learn"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" learn"],
    ),
    # 56
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" talk"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " talk"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" talk"],
    ),
    # 57
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"."],
    ),
    # 58
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" new"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " new"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" new"],
    ),
    # 59
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"."],
    ),
    # 60
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" When"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " When"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" When"],
    ),
    # 61
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" things"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " things"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" things"],
    ),
    # 62
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" This"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " This"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" This"],
    ),
    # 63
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 64
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"."],
    ),
    # 65
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" robot"],
    ),
    # 66
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" want"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " want"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" want"],
    ),
    # 67
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" This"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " This"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" This"],
    ),
    # 68
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" has"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " has"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" has"],
    ),
    # 69
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 70
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" robot"],
    ),
    # 71
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" read"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " read"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" read"],
    ),
    # 72
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" draw"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " draw"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" draw"],
    ),
    # 73
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " is"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" is"],
    ),
    # 74
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" a"],
    ),
    # 75
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" something"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " something"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" something"],
    ),
    # 76
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" called"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " called"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" called"],
    ),
    # 77
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" lot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " lot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" lot"],
    ),
    # 78
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 79
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " an"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" an"],
    ),
    # 80
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" of"],
    ),
    # 81
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 82
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" L"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " L"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" L"],
    ),
    # 83
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" books"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " books"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" books"],
    ),
    # 84
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" pick"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " pick"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" pick"],
    ),
    # 85
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"LM"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "LM"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"LM"],
    ),
    # 86
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b","],
    ),
    # 87
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" the"],
    ),
    # 88
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 89
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" stories"],
    ),
    # 90
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" crayons"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " crayons"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" crayons"],
    ),
    # 91
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" which"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " which"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" which"],
    ),
    # 92
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b","],
    ),
    # 93
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 94
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" stands"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " stands"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" stands"],
    ),
    # 95
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" and"],
    ),
    # 96
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" need"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " need"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" need"],
    ),
    # 97
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" for"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " for"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" for"],
    ),
    # 98
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" all"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " all"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" all"],
    ),
    # 99
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 100
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" \\""},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " \\""}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b' "'],
    ),
    # 101
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" kinds"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " kinds"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" kinds"],
    ),
    # 102
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" make"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " make"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" make"],
    ),
    # 103
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"Large"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "Large"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"Large"],
    ),
    # 104
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" of"],
    ),
    # 105
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " your"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" your"],
    ),
    # 106
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" Language"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " Language"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" Language"],
    ),
    # 107
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" words"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " words"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" words"],
    ),
    # 108
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" picture"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " picture"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" picture"],
    ),
    # 109
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" Model"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " Model"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" Model"],
    ),
    # 110
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b","],
    ),
    # 111
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"."],
    ),
    # 112
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":".\\""},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ".\\""}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b'."'],
    ),
    # 113
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" so"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " so"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" so"],
    ),
    # 114
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" \\n\\n"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " \\n\\n"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" \n\n"],
    ),
    # 115
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" \\n\\n"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " \\n\\n"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" \n\n"],
    ),
    # 116
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 117
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"Now"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "Now"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Now"],
    ),
    # 118
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"Just"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "Just"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"Just"],
    ),
    # 119
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" knows"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " knows"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" knows"],
    ),
    # 120
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 121
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" like"],
    ),
    # 122
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" a"],
    ),
    # 123
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" think"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " think"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" think"],
    ),
    # 124
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 125
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" lot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " lot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" lot"],
    ),
    # 126
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" of"],
    ),
    # 127
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" learn"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " learn"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" learn"],
    ),
    # 128
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" about"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " about"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" about"],
    ),
    # 129
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" L"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " L"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" L"],
    ),
    # 130
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" words"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " words"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" words"],
    ),
    # 131
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" many"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " many"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" many"],
    ),
    # 132
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"LM"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "LM"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"LM"],
    ),
    # 133
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 134
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" things"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " things"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" things"],
    ),
    # 135
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 136
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" how"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " how"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" how"],
    ),
    # 137
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"."],
    ),
    # 138
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" which"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " which"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" which"],
    ),
    # 139
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 140
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" \\n\\n"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " \\n\\n"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" \n\n"],
    ),
    # 141
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" stands"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " stands"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" stands"],
    ),
    # 142
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" put"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " put"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" put"],
    ),
    # 143
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"When"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "When"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"When"],
    ),
    # 144
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" for"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " for"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" for"],
    ),
    # 145
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" them"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " them"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" them"],
    ),
    # 146
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" you"],
    ),
    # 147
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" \\""},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " \\""}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b' "'],
    ),
    # 148
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" together"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " together"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" together"],
    ),
    # 149
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" ask"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " ask"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" ask"],
    ),
    # 150
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"Large"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "Large"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Large"],
    ),
    # 151
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 152
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 153
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" Language"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " Language"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" Language"],
    ),
    # 154
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" make"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " make"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" make"],
    ),
    # 155
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" a"],
    ),
    # 156
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" Model"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " Model"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" Model"],
    ),
    # 157
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" sentences"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " sentences"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" sentences"],
    ),
    # 158
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" question"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " question"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" question"],
    ),
    # 159
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":",\\""},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ",\\""}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b',"'],
    ),
    # 160
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 161
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" or"],
    ),
    # 162
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" like"],
    ),
    # 163
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" the"],
    ),
    # 164
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" talk"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " talk"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" talk"],
    ),
    # 165
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 166
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" L"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " L"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" L"],
    ),
    # 167
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" to"],
    ),
    # 168
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" super"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " super"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" super"],
    ),
    # 169
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"LM"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "LM"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"LM"],
    ),
    # 170
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 171
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" smart"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " smart"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" smart"],
    ),
    # 172
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" has"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " has"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" has"],
    ),
    # 173
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b","],
    ),
    # 174
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" robot"],
    ),
    # 175
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" read"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " read"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" read"],
    ),
    # 176
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 177
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" that"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " that"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" that"],
    ),
    # 178
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" lots"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " lots"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" lots"],
    ),
    # 179
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" uses"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " uses"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" uses"],
    ),
    # 180
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" knows"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " knows"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" knows"],
    ),
    # 181
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 182
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" all"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " all"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" all"],
    ),
    # 183
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 184
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" lots"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " lots"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" lots"],
    ),
    # 185
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" the"],
    ),
    # 186
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" lot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " lot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" lot"],
    ),
    # 187
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" of"],
    ),
    # 188
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" things"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " things"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" things"],
    ),
    # 189
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" about"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " about"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" about"],
    ),
    # 190
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" books"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " books"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" books"],
    ),
    # 191
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 192
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" words"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " words"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" words"],
    ),
    # 193
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 194
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" has"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " has"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" has"],
    ),
    # 195
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" and"],
    ),
    # 196
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" articles"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " articles"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" articles"],
    ),
    # 197
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" learned"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " learned"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" learned"],
    ),
    # 198
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" stories"],
    ),
    # 199
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 200
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" to"],
    ),
    # 201
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"."],
    ),
    # 202
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 203
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" give"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " give"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" give"],
    ),
    # 204
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" It"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " It"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" It"],
    ),
    # 205
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" stories"],
    ),
    # 206
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" you"],
    ),
    # 207
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" has"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " has"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" has"],
    ),
    # 208
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"."],
    ),
    # 209
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" answers"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " answers"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" answers"],
    ),
    # 210
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" read"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " read"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" read"],
    ),
    # 211
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" So"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " So"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" So"],
    ),
    # 212
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" or"],
    ),
    # 213
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 214
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 215
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" tell"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " tell"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" tell"],
    ),
    # 216
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" huge"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " huge"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" huge"],
    ),
    # 217
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" it"],
    ),
    # 218
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" you"],
    ),
    # 219
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" number"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " number"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" number"],
    ),
    # 220
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" knows"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " knows"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" knows"],
    ),
    # 221
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" stories"],
    ),
    # 222
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" of"],
    ),
    # 223
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 224
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"."],
    ),
    # 225
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" books"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " books"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" books"],
    ),
    # 226
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" lot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " lot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" lot"],
    ),
    # 227
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" It"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " It"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" It"],
    ),
    # 228
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 229
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" about"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " about"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" about"],
    ),
    # 230
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "\u2019s"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"\xe2\x80\x99s"],
    ),
    # 231
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" just"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " just"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" just"],
    ),
    # 232
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" how"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " how"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" how"],
    ),
    # 233
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" like"],
    ),
    # 234
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" like"],
    ),
    # 235
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 236
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" having"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " having"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" having"],
    ),
    # 237
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" how"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " how"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" how"],
    ),
    # 238
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" talk"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " talk"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" talk"],
    ),
    # 239
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" a"],
    ),
    # 240
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 241
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 242
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" super"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " super"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" super"],
    ),
    # 243
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" learn"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " learn"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" learn"],
    ),
    # 244
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" write"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " write"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" write"],
    ),
    # 245
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" helper"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " helper"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" helper"],
    ),
    # 246
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 247
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"!\\n\\n"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "!\\n\\n"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"!\n\n"],
    ),
    # 248
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" who"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " who"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" who"],
    ),
    # 249
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" draw"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " draw"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" draw"],
    ),
    # 250
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"When"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "When"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"When"],
    ),
    # 251
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " is"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" is"],
    ),
    # 252
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" by"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " by"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" by"],
    ),
    # 253
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 254
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" always"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " always"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" always"],
    ),
    # 255
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" practicing"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " practicing"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" practicing"],
    ),
    # 256
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" ask"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " ask"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" ask"],
    ),
    # 257
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" ready"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " ready"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" ready"],
    ),
    # 258
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " with"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" with"],
    ),
    # 259
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" the"],
    ),
    # 260
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" to"],
    ),
    # 261
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " your"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" your"],
    ),
    # 262
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" robot"],
    ),
    # 263
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" chat"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " chat"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" chat"],
    ),
    # 264
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" crayons"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " crayons"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" crayons"],
    ),
    # 265
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 266
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" and"],
    ),
    # 267
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"."],
    ),
    # 268
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" question"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " question"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" question"],
    ),
    # 269
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" share"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " share"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" share"],
    ),
    # 270
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" \\n\\n"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " \\n\\n"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" \n\n"],
    ),
    # 271
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" or"],
    ),
    # 272
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" what"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " what"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" what"],
    ),
    # 273
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"When"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "When"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"When"],
    ),
    # 274
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" tell"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " tell"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" tell"],
    ),
    # 275
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" it"],
    ),
    # 276
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 277
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" it"],
    ),
    # 278
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":" knows"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": " knows"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b" knows"],
    ),
    # 279
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" ask"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " ask"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" ask"],
    ),
    # 280
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 281
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"", b"!"],
    ),
    # 282
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" the"],
    ),
    # 283
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" make"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " make"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" make"],
    ),
    # 284
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" robot"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " robot"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" robot"],
    ),
    # 285
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 286
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 287
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" story"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " story"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" story"],
    ),
    # 288
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" question"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " question"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" question"],
    ),
    # 289
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 290
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" or"],
    ),
    # 291
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" it"],
    ),
    # 292
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" tell"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " tell"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" tell"],
    ),
    # 293
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" thinks"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " thinks"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" thinks"],
    ),
    # 294
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" it"],
    ),
    # 295
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" really"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " really"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" really"],
    ),
    # 296
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 297
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" fast"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " fast"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" fast"],
    ),
    # 298
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" make"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " make"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" make"],
    ),
    # 299
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" and"],
    ),
    # 300
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" up"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " up"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" up"],
    ),
    # 301
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" uses"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " uses"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" uses"],
    ),
    # 302
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 303
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" all"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " all"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" all"],
    ),
    # 304
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" story"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " story"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" story"],
    ),
    # 305
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" the"],
    ),
    # 306
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 307
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" things"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " things"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" things"],
    ),
    # 308
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" it"],
    ),
    # 309
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" it"],
    ),
    # 310
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" looks"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " looks"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" looks"],
    ),
    # 311
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" learned"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " learned"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" learned"],
    ),
    # 312
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " in"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" in"],
    ),
    # 313
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 314
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" its"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " its"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" its"],
    ),
    # 315
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" give"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " give"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" give"],
    ),
    # 316
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" big"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " big"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" big"],
    ),
    # 317
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 318
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" memory"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " memory"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" memory"],
    ),
    # 319
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " an"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" an"],
    ),
    # 320
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" ("},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " ("}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" ("],
    ),
    # 321
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" answer"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " answer"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" answer"],
    ),
    # 322
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"like"],
    ),
    # 323
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" or"],
    ),
    # 324
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " your"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" your"],
    ),
    # 325
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" tell"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " tell"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" tell"],
    ),
    # 326
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" box"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " box"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" box"],
    ),
    # 327
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 328
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " of"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" of"],
    ),
    # 329
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 330
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" crayons"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " crayons"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" crayons"],
    ),
    # 331
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" fun"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " fun"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" fun"],
    ),
    # 332
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":")"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ")"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b")"],
    ),
    # 333
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" tale"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " tale"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" tale"],
    ),
    # 334
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " and"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" and"],
    ),
    # 335
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"."],
    ),
    # 336
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" picks"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " picks"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" picks"],
    ),
    # 337
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" It"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " It"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" It"],
    ),
    # 338
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " the"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" the"],
    ),
    # 339
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" doesn"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " doesn"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" doesn"],
    ),
    # 340
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" right"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " right"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" right"],
    ),
    # 341
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"\xe2\x80\x99t"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "\u2019t"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"\xe2\x80\x99t"],
    ),
    # 342
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" words"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " words"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" words"],
    ),
    # 343
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" know"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " know"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" know"],
    ),
    # 344
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 345
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" everything"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " everything"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" everything"],
    ),
    # 346
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" help"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " help"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" help"],
    ),
    # 347
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 348
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 349
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" but"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " but"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" but"],
    ),
    # 350
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "."}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"."],
    ),
    # 351
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" it"],
    ),
    # 352
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" So"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " So"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" So"],
    ),
    # 353
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" tries"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " tries"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" tries"],
    ),
    # 354
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 355
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" its"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " its"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" its"],
    ),
    # 356
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " it"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" it"],
    ),
    # 357
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" best"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " best"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" best"],
    ),
    # 358
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" can"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " can"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" can"],
    ),
    # 359
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" to"],
    ),
    # 360
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" talk"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " talk"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" talk"],
    ),
    # 361
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" help"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " help"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" help"],
    ),
    # 362
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 363
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 364
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 365
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " with"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" with"],
    ),
    # 366
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 367
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" whatever"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " whatever"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" whatever"],
    ),
    # 368
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" answer"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " answer"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" answer"],
    ),
    # 369
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 370
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " your"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" your"],
    ),
    # 371
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" ask"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " ask"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" ask"],
    ),
    # 372
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" questions"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " questions"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" questions"],
    ),
    # 373
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"!"],
    ),
    # 374
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 375
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" It"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " It"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" It"],
    ),
    # 376
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" or"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " or"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" or"],
    ),
    # 377
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "\u2019s"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"\xe2\x80\x99s"],
    ),
    # 378
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" tell"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " tell"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" tell"],
    ),
    # 379
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" like"],
    ),
    # 380
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 381
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" having"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " having"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" having"],
    ),
    # 382
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" fun"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " fun"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" fun"],
    ),
    # 383
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " a"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" a"],
    ),
    # 384
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" stories"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " stories"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" stories"],
    ),
    # 385
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" super"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " super"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" super"],
    ),
    # 386
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 387
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" smart"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " smart"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" smart"],
    ),
    # 388
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" just"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " just"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" just"],
    ),
    # 389
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":","},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": ","}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b","],
    ),
    # 390
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " like"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" like"],
    ),
    # 391
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" talking"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " talking"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" talking"],
    ),
    # 392
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 393
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" book"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " book"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" book"],
    ),
    # 394
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" use"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " use"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" use"],
    ),
    # 395
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" that"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " that"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" that"],
    ),
    # 396
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " your"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" your"],
    ),
    # 397
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" can"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " can"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" can"],
    ),
    # 398
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" crayons"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " crayons"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" crayons"],
    ),
    # 399
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" chat"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " chat"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" chat"],
    ),
    # 400
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " to"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 401
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " with"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" with"],
    ),
    # 402
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" make"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " make"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" make"],
    ),
    # 403
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": " you"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b" you"],
    ),
    # 404
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" colorful"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " colorful"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" colorful"],
    ),
    # 405
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"", b"!"],
    ),
    # 406
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":" pictures"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": " pictures"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" pictures"],
    ),
    # 407
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {"content": "!"}, "logprobs": null, "finish_reason": null}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"!"],
    ),
    # 408
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 0, "delta": {}, "logprobs": null, "finish_reason": "stop"}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=None,
    ),
    # 409
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":1,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 1, "delta": {}, "logprobs": null, "finish_reason": "stop"}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=None,
    ),
    # 410
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[{"index":2,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":null}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [{"index": 2, "delta": {}, "logprobs": null, "finish_reason": "stop"}], "usage": null}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=None,
    ),
    # 411
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd","object":"chat.completion.chunk","created":1738272482,"model":"gpt-4o-mini-2024-07-18","service_tier":"default","system_fingerprint":"fp_72ed7ab54c","choices":[],"usage":{"prompt_tokens":23,"completion_tokens":408,"total_tokens":431,"prompt_tokens_details":{"cached_tokens":0,"audio_tokens":0},"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":0,"accepted_prediction_tokens":0,"rejected_prediction_tokens":0}}}\n\n',
        json_data=json.loads(
            '{"id": "chatcmpl-AvWMsDyiI0PE2RQOQOXmabPfnFsWd", "object": "chat.completion.chunk", "created": 1738272482, "model": "gpt-4o-mini-2024-07-18", "service_tier": "default", "system_fingerprint": "fp_72ed7ab54c", "choices": [], "usage": {"prompt_tokens": 23, "completion_tokens": 408, "total_tokens": 431, "prompt_tokens_details": {"cached_tokens": 0, "audio_tokens": 0}, "completion_tokens_details": {"reasoning_tokens": 0, "audio_tokens": 0, "accepted_prediction_tokens": 0, "rejected_prediction_tokens": 0}}}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=None,
    ),
    # 412
    StreamChunkData(
        raw_data=b"data: [DONE]",
        json_data=None,
        type=StreamChunkDataType.DONE,
        contents=None,
    ),
]

utf8_test_chunk_data: List[StreamChunkData] = [
    # 0
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b""],
    ),
    # 1
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Of"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Of"],
    ),
    # 2
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" course"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" course"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" course"],
    ),
    # 3
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b","],
    ),
    # 4
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" here\'s"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" here\'s"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" here's"],
    ),
    # 5
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" another"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" another"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" another"],
    ),
    # 6
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" one"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" one"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" one"],
    ),
    # 7
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" for"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" for"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" for"],
    ),
    # 8
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" you"],
    ),
    # 9
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":":\\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":":\\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b":\n\n"],
    ),
    # 10
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Why"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Why"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Why"],
    ),
    # 11
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" do"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" do"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" do"],
    ),
    # 12
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" programmers"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" programmers"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" programmers"],
    ),
    # 13
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" prefer"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" prefer"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" prefer"],
    ),
    # 14
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" using"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" using"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" using"],
    ),
    # 15
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" the"],
    ),
    # 16
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" keyboard"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" keyboard"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" keyboard"],
    ),
    # 17
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"?\\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"?\\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"?\n\n"],
    ),
    # 18
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Because"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"Because"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"Because"],
    ),
    # 19
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" they"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" they"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" they"],
    ),
    # 20
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" don"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" don"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" don"],
    ),
    # 21
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99t"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99t"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"\xe2\x80\x99t"],
    ),
    # 22
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" want"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" want"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" want"],
    ),
    # 23
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" to"],
    ),
    # 24
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" deal"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" deal"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" deal"],
    ),
    # 25
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" with"],
    ),
    # 26
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" a"],
    ),
    # 27
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" **"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" **"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" **"],
    ),
    # 28
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"mouse"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"mouse"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"mouse \xf0\x9f\x96\xb1"],
    ),
    # 29
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" trap"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" trap"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b" trap"],
    ),
    # 30
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"**"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"**"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"**"],
    ),
    # 31
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"!  "],
    ),
    # 32
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" \xf0\x9f\x96\xb1"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":" \xf0\x9f\x96\xb1"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"   \xf0\x9f\x96\xb1"],
    ),
    # 33
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xef\xb8\x8f"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xef\xb8\x8f"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"\xef\xb8\x8f"],
    ),
    # 34
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xf0\x9f\x98\x82"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{"content":"\xf0\x9f\x98\x82"},"logprobs":null,"finish_reason":null}]}'
        ),
        type=StreamChunkDataType.NORMAL_TEXT,
        contents=[b"\xf0\x9f\x98\x82"],
    ),
    # 35
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AtIDcvSoOQYUP1FigEYZfNMXIL2Fo","object":"chat.completion.chunk","created":1737741436,"model":"chatgpt-4o-latest","service_tier":"default","system_fingerprint":"fp_60a3f2dc65","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}'
        ),
        type=StreamChunkDataType.FINISH_NO_CONTENT,
        contents=None,
    ),
    # 36
    StreamChunkData(
        raw_data=b"data: [DONE]\n\n",
        json_data=None,
        type=StreamChunkDataType.DONE,
        contents=None,
    ),
]

test_chunk_data: List[StreamChunkData] = [
    # 0
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"role":"assistant","content":"","refusal":null},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b""],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 1
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"In"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 2
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 3
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" heart"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" heart"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" heart"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 4
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 5
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 6
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" code"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" code"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" code"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 7
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 8
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 9
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" dance"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" dance"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" dance"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 10
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" unfolds"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" unfolds"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" unfolds"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 11
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 12
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 13
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Where"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Where"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Where"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 14
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" whispers"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" whispers"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" whispers"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 15
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 16
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" logic"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" logic"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" logic"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 17
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 18
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 19
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" layers"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" layers"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" layers"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 20
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 21
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" are"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" are"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" are"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 22
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" told"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" told"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" told"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 23
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 24
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 25
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"A"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 26
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" mystery"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" mystery"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" mystery"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 27
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" wrapped"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" wrapped"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" wrapped"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 28
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 29
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" loops"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" loops"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" loops"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 30
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" that"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" that"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" that"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 31
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" entw"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" entw"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" entw"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 32
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ine"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ine"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"ine"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 33
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 34
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 35
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Rec"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Rec"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Rec"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 36
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ursion"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ursion"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"ursion"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 37
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 38
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" dear"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" dear"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" dear"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 39
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" friend"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" friend"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" friend"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 40
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 41
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" is"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 42
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" both"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" both"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" both"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 43
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" simple"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" simple"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" simple"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 44
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" and"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" and"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 45
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" fine"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" fine"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" fine"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 46
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 47
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 48
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Imagine"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Imagine"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Imagine"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 49
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 50
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" mirror"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" mirror"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" mirror"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 51
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 52
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" reflecting"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" reflecting"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" reflecting"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 53
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 54
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" face"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" face"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" face"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 55
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 56
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 57
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Each"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 58
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" glance"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" glance"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" glance"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 59
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" leads"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" leads"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" leads"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 60
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" to"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 61
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" more"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" more"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" more"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 62
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 63
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 64
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" an"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 65
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" infinite"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" infinite"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" infinite"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 66
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" space"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" space"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" space"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 67
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 68
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 69
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\""},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\""},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b'"'],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 70
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Call"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Call"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Call"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 71
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" me"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" me"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" me"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    # 72
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" again"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" again"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" again"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":",\\""},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":",\\""},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b',"'],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" says"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" says"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" says"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" function"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" function"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" function"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" with"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" g"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" g"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" g"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"lee"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"lee"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"lee"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\"I"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\\"I"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b'"I'],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\'ll"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\'ll"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"'ll"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" solve"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" solve"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" solve"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" this"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" small"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" small"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" small"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" problem"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" problem"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" problem"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" then"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" then"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" then"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" set"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" set"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" set"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" you"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" you"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" free"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" free"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" free"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":".\\""},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":".\\""},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b'."'],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"The"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"The"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"The"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" base"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" base"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" base"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" case"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" case"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" case"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" is"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" is"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" vital"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" vital"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" vital"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" stop"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" stop"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" stop"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" flow"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" flow"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" flow"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"A"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" moment"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" moment"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" moment"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" pause"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" pause"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" pause"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" where"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" where"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" where"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" finally"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" finally"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" finally"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" know"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" know"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" know"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x94"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x94"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\xe2\x80\x94"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"When"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"When"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"When"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" should"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" should"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" should"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" halt"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" halt"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" halt"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" when"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" when"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" when"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" should"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" should"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" should"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" refrain"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" refrain"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" refrain"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"?"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"?"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"?"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Without"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Without"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Without"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" it"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99d"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99d"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\xe2\x80\x99d"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" spiral"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" spiral"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" spiral"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" an"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" an"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" endless"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" endless"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" endless"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" refrain"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" refrain"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" refrain"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"A"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"A"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" factorial"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" factorial"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" factorial"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\xe2\x80\x99s"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" journey"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" journey"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" journey"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" from"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" from"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" from"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" five"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" five"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" five"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" down"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" down"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" down"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" to"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" one"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" one"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" one"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Each"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" step"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" step"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" step"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" its"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" its"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" its"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" path"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" path"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" path"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" new"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" new"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" new"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" call"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" call"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" call"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" begun"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" begun"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" begun"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Five"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Five"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Five"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" times"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" times"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" times"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" four"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" four"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" four"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" times"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" times"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" times"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" three"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" three"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" three"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x94"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x94"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\xe2\x80\x94"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"oh"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"oh"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"oh"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" what"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" what"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" what"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" delight"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" delight"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" delight"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"!"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Each"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Each"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" function"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" function"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" function"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" returns"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" returns"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" returns"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" like"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" like"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" stars"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" stars"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" stars"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" back"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" back"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" back"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" to"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" to"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" night"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" night"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" night"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"In"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" trees"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" trees"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" trees"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" decisions"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" decisions"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" decisions"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" recursion"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" recursion"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" recursion"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" can"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" can"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" can"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" bloom"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" bloom"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" bloom"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Branch"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Branch"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Branch"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ing"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"ing"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"ing"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" out"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" out"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" out"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" softly"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" softly"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" softly"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" disp"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" disp"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" disp"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"elling"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"elling"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"elling"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" gloom"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" gloom"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" gloom"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"With"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"With"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"With"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" each"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" each"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" each"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" nested"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" nested"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" nested"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" call"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" call"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" call"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" traverse"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" traverse"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" traverse"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" with"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" grace"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" grace"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" grace"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Re"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Re"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Re"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"vis"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"vis"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"vis"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"iting"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"iting"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"iting"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" paths"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" paths"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" paths"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" in"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" in"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" this"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" elegant"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" elegant"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" elegant"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" space"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" space"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" space"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n\\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n\n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"So"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"So"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"So"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" fear"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" fear"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" fear"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" not"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" not"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" not"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" depth"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" depth"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" depth"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" nor"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" nor"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" nor"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" layers"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" layers"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" layers"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" so"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" so"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" so"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" vast"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" vast"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" vast"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"For"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"For"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"For"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" with"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" with"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" eager"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" eager"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" eager"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" recursion"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" recursion"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" recursion"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" we"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" we"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" solve"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" solve"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" solve"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" cast"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" cast"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" cast"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"."},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"."],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Em"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"Em"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"Em"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"brace"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"brace"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"brace"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" this"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" this"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" technique"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" technique"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" technique"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" let"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" let"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" let"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" it"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" guide"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" guide"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" guide"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" your"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" your"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" way"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" way"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" way"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  \\n"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  \n"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"In"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"In"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" realm"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" realm"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" realm"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" of"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" of"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" the"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" the"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" code"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" code"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" code"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" it"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" it"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"\xe2\x80\x99s"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"\xe2\x80\x99s"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" a"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" a"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" bright"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" bright"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" bright"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":","},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b","],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" shining"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" shining"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" shining"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" day"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":" day"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b" day"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"!"},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"!"],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  "},"logprobs":null,"finish_reason":null}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{"content":"  "},"logprobs":null,"finish_reason":null}]}'
        ),
        contents=[b"  "],
        type=StreamChunkDataType.NORMAL_TEXT,
    ),
    StreamChunkData(
        raw_data=b'data: {"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}\n\n',
        json_data=json.loads(
            '{"id":"chatcmpl-AVPZexJ0frlpPaMIa2MsW9a99fA1z","object":"chat.completion.chunk","created":1732049838,"model":"gpt-4o-mini-2024-07-18","system_fingerprint":"fp_3de1288069","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}'
        ),
        contents=None,
        type=StreamChunkDataType.FINISH_NO_CONTENT,
    ),
    StreamChunkData(
        raw_data=b"data: [DONE]\n\n",
        json_data=None,
        contents=None,
        type=StreamChunkDataType.DONE,
    ),
]
