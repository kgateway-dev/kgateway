from ai_extension.ext_proc.provider import num_tokens_from_messages


class TestNumTokensFromMessages:
    def test_empty_message(self):
        messages = [{}]
        assert num_tokens_from_messages(messages) == 6

    def test_simple_message(self):
        messages = [{"role": "user", "content": "Hello, world!"}]
        assert num_tokens_from_messages(messages) == 13

    def test_tool_call(self):
        messages = [
            {
                "id": "call_b0BUgjUoOPkXRseHdkeZm7xL",
                "function": {
                    "arguments": '{"location": "San Francisco, CA"}',
                    "name": "get_current_weather",
                },
                "type": "function",
            }
        ]
        assert num_tokens_from_messages(messages) == 45

    def test_all_messages_nofuncs(self):
        messages = [
            {"role": "user", "content": "Hello, how are you?"},
            {
                "role": "system",
                "name": "Claude",
                "content": "I'm doing well, thank you for asking!",
            },
            {
                "role": "user",
                "content": "Here are my favorite colors: blue green red",
            },
        ]
        assert num_tokens_from_messages(messages) == 50

    def test_all_messages(self):
        messages = [
            {"role": "user", "content": "Hello, how are you?"},
            {
                "role": "assistant",
                "name": "Claude",
                "content": "I'm doing well, thank you for asking!",
            },
            {
                "role": "assistant",
                "content": None,
                "function_call": {
                    "name": "get_weather",
                    "arguments": '{"location": "New York", "unit": "celsius"}',
                },
            },
            {
                "role": "user",
                "content": "Here are my favorite colors:",
                "colors": ["red", "blue", "green"],
            },
            {
                "role": "assistant",
                "content": "Here's a summary of your order:",
                "order": {
                    "items": [
                        {"name": "Widget", "quantity": 5, "price": 9.99},
                        {"name": "Gadget", "quantity": 2, "price": 15.50},
                    ],
                    "total": 80.95,
                    "shipping": {
                        "method": "express",
                        "address": {
                            "street": "123 Main St",
                            "city": "Anytown",
                            "country": "USA",
                        },
                    },
                },
            },
            {},
            {"timestamp": 1234567890, "temperature": 22.5, "is_sunny": True},
            {
                "id": "call_b0BUgjUoOPkXRseHdkeZm7xL",
                "function": {
                    "arguments": '{"location": "San Francisco, CA"}',
                    "name": "get_current_weather",
                },
                "type": "function",
            },
        ]
        assert num_tokens_from_messages(messages) == 172
