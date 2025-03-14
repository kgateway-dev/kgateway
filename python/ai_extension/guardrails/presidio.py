import regex as re
from api.kgateway.policy.ai import prompt_guard
from presidio_analyzer import (
    EntityRecognizer,
    PatternRecognizer,
    Pattern,
)
from presidio_analyzer.predefined_recognizers import (
    PhoneRecognizer,
    UsSsnRecognizer,
    CreditCardRecognizer,
    EmailRecognizer,
)

global_regex_flage = re.DOTALL | re.MULTILINE | re.IGNORECASE


def init_presidio_config(
    guardrails_regex: prompt_guard.Regex,
) -> list[EntityRecognizer]:
    recognizers: list[EntityRecognizer] = []
    compiled_regex: list[Pattern] = []
    for builtin in guardrails_regex.builtins:
        match builtin:
            case prompt_guard.BuiltIn.CREDIT_CARD:
                recognizers.append(CreditCardRecognizer())
            case prompt_guard.BuiltIn.SSN:
                recognizers.append(UsSsnRecognizer())
            case prompt_guard.BuiltIn.PHONE_NUMBER:
                recognizers.append(PhoneRecognizer())
            case prompt_guard.BuiltIn.EMAIL:
                recognizers.append(EmailRecognizer())
    for idx, regex_match in enumerate(guardrails_regex.matches):
        compiled_re = re.compile(regex_match.pattern, global_regex_flage)
        pattern = Pattern(
            regex_match.name if regex_match.name != "" else f"regex_{idx}",
            regex_match.pattern,
            1.0,  # Set the score to 1.0 for now, this indicates that a match is always 100% accurate
        )
        pattern.compiled_regex = compiled_re  # type: ignore
        pattern.compiled_with_flags = global_regex_flage  # type: ignore
        compiled_regex.append(pattern)
    if len(compiled_regex) > 0:
        recognizer = PatternRecognizer(
            supported_entity="CUSTOM", name="custom", patterns=compiled_regex
        )
        recognizers.append(recognizer)
    return recognizers
