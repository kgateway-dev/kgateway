from api.kgateway.policy.ai import prompt_guard
from presidio_analyzer import EntityRecognizer
from presidio_anonymizer.entities import RecognizerResult
from presidio_anonymizer import AnonymizerEngine


def regex_transform(
    role: str,
    content: str,
    rules: list[EntityRecognizer] | None,
    anon: AnonymizerEngine,
    action: prompt_guard.Action = prompt_guard.Action.MASK,
) -> str:
    if rules:
        matrix = [
            i.analyze(content, [], nlp_artifacts=None)  # type: ignore
            for i in rules
        ]
        results = [item for row in matrix for item in row]
        EntityRecognizer.remove_duplicates(results)
        # if we have results and the action is to reject, raise an error
        if len(results) > 0 and action == prompt_guard.Action.REJECT:
            raise RegexRejection(" ".join([str(i) for i in results]))

        anonymized = anon.anonymize(
            text=content,
            analyzer_results=[
                RecognizerResult(
                    entity_type=i.entity_type,
                    start=i.start,
                    end=i.end,
                    score=i.score,
                )
                for i in results
            ],
        )
        return anonymized.text
    else:
        return content


class RegexRejection(Exception):
    """
    RegexRejection is an exception that is raised when the regex action is set to REJECT.
    """

    def __init__(self, message: str):
        self.message = message

    def __str__(self) -> str:
        return self.message
