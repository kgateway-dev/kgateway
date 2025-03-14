import regex as re
import datetime
from collections import defaultdict
from typing import List, Optional
from presidio_analyzer import (
    PatternRecognizer,
    Pattern,
)
from presidio_analyzer.predefined_recognizers import PhoneRecognizer, UsSsnRecognizer


class MyUsSsnRecognizer(PatternRecognizer):
    """Recognize US Social Security Number (SSN) using regex.

    :param patterns: List of patterns to be used by this recognizer
    :param context: List of context words to increase confidence in detection
    :param supported_language: Language this recognizer supports
    :param supported_entity: The entity this recognizer can detect
    """

    PATTERNS = [
        Pattern("SSN1 (very weak)", r"\b([0-9]{5})-([0-9]{4})\b", 0.05),  # noqa E501
        Pattern("SSN2 (very weak)", r"\b([0-9]{3})-([0-9]{6})\b", 0.05),  # noqa E501
        Pattern("SSN3 (very weak)", r"\b(([0-9]{3})-([0-9]{2})-([0-9]{4}))\b", 0.05),  # noqa E501
        Pattern("SSN4 (very weak)", r"\b[0-9]{9}\b", 0.05),
        Pattern("SSN5 (medium)", r"\b([0-9]{3})[- .]([0-9]{2})[- .]([0-9]{4})\b", 0.5),
    ]

    CONTEXT = [
        "social",
        "security",
        # "sec", # Task #603: Support keyphrases ("social sec")
        "ssn",
        "ssns",
        "ssn#",
        "ss#",
        "ssid",
    ]

    def __init__(
        self,
        patterns: Optional[List[Pattern]] = None,
        context: Optional[List[str]] = None,
        supported_language: str = "en",
        supported_entity: str = "US_SSN",
    ):
        patterns = patterns if patterns else self.PATTERNS
        context = context if context else self.CONTEXT
        super().__init__(
            supported_entity=supported_entity,
            patterns=patterns,
            context=context,
            supported_language=supported_language,
        )

    def invalidate_result(self, pattern_text: str) -> bool:
        """
        Check if the pattern text cannot be validated as a US_SSN entity.

        :param pattern_text: Text detected as pattern by regex
        :return: True if invalidated
        """
        # if there are delimiters, make sure both delimiters are the same
        delimiter_counts = defaultdict(int)
        for c in pattern_text:
            if c in (".", "-", " "):
                delimiter_counts[c] += 1
        if len(delimiter_counts.keys()) > 1:
            # mismatched delimiters
            return True

        only_digits = "".join(c for c in pattern_text if c.isdigit())
        if all(only_digits[0] == c for c in only_digits):
            # cannot be all same digit
            return True

        if only_digits[3:5] == "00" or only_digits[5:] == "0000":
            # groups cannot be all zeros
            return True

        for sample_ssn in ("000", "666", "123456789", "98765432", "078051120"):
            if only_digits.startswith(sample_ssn):
                return True

        return False


# Set up the engine, loads the NLP module (spaCy model by default) and other PII recognizers

phone = PhoneRecognizer()
match_start_time = datetime.datetime.now()
print(phone.analyze(text="My phone number is 212-555-5555", entities=["PHONE_NUMBER"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

ssn = UsSsnRecognizer()
match_start_time = datetime.datetime.now()
print(ssn.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

ssn = MyUsSsnRecognizer()
match_start_time = datetime.datetime.now()
print(ssn.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

compiled = re.compile(
    r"\b([0-9]{3})[- .]([0-9]{2})[- .]([0-9]{4})\b",
    re.DOTALL | re.MULTILINE | re.IGNORECASE,
)
numbers_pattern = Pattern(
    name="numbers_pattern",
    regex=r"\b([0-9]{3})[- .]([0-9]{2})[- .]([0-9]{4})\b",
    score=0.5,
)
numbers_pattern.compiled_regex = compiled
numbers_pattern.compiled_with_flags = re.DOTALL | re.MULTILINE | re.IGNORECASE

# Define the recognizer with one or more patterns
number_recognizer = MyUsSsnRecognizer(
    supported_entity="US_SSN",
    # patterns=UsSsnRecognizer.PATTERNS,
    # context=UsSsnRecognizer.CONTEXT,
    supported_language="en",
)

# Create the registry and add the recognizers
match_start_time = datetime.datetime.now()
print(number_recognizer.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

# Create the registry and add the recognizers
match_start_time = datetime.datetime.now()
print(number_recognizer.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

# Create the registry and add the recognizers
match_start_time = datetime.datetime.now()
print(number_recognizer.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")

# Create the registry and add the recognizers
match_start_time = datetime.datetime.now()
print(number_recognizer.analyze(text="My SSN is 123-45-6787", entities=["US_SSN"]))
match_time = datetime.datetime.now() - match_start_time
print(f"--- match_time: {match_time.seconds}.{match_time.microseconds} seconds")
