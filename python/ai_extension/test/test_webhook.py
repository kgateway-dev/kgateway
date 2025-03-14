import unittest

from typing import List
from guardrails.api import Message, ResponseChoice, ResponseChoices
from guardrails.webhook import extract_contents_from_response_webhook_response


class TestWebhookHelpers(unittest.TestCase):
    def test_extract_contents_from_response_webhook_response(self):
        expected_contents: List[str] = [
            "1 + 2 = 3",
            "The result of adding 1 to 2 is 3.",
        ]
        rc = ResponseChoices()
        rc.choices.append(
            ResponseChoice(message=Message(role="ai", content=expected_contents[0]))
        )
        rc.choices.append(
            ResponseChoice(message=Message(role="ai", content=expected_contents[1]))
        )

        contents = extract_contents_from_response_webhook_response(rc)
        assert contents == expected_contents
