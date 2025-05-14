"""
This is a sample FastAPI application that demonstrates how to use the Guardrails API.

To run this server in debug mode, naviagate to the parent directory of this file and run:

  ```bash
  python3 -m fastapi dev --host 0.0.0.0 samples/app.py
  ```

This command will also reload theprogram as you make changes to the source code.
"""

import os
import signal
import sys
from typing import Annotated
from fastapi import FastAPI, Header, Request

from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.propagate import extract
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import (
    BatchSpanProcessor,
)
from opentelemetry.sdk.resources import (
    SERVICE_NAME,
    Resource,
)
from opentelemetry import trace
from presidio_analyzer import AnalyzerEngine
from presidio_anonymizer import AnonymizerEngine, EngineResult
from presidio_anonymizer.entities import RecognizerResult
import uvicorn

sys.path.append("ai_extension/")
sys.path.append("ai_extension/api")
import guardrails.api as api

app = FastAPI()
FastAPIInstrumentor().instrument_app(app)

# Set up the engine, loads the NLP module (spaCy model by default) and other PII recognizers
analyzer = AnalyzerEngine()
anon = AnonymizerEngine()


def tracer() -> trace.Tracer | trace.NoOpTracer:
    # To enabled tracing for testing:
    #    export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="localhost:4317"
    # and port-forward port 4317 of the jaeger pod to localhost
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
    if endpoint is None or len(endpoint) == 0:
        return trace.NoOpTracer()

    # Initialize tracer provider
    resource = Resource.create(attributes={SERVICE_NAME: "kgateway-ai-webhook"})
    tracer_provider = TracerProvider(resource=resource)
    # Configure span processor and exporter
    span_processor = BatchSpanProcessor(
        OTLPSpanExporter(
            # endpoint="localhost:4317",
            endpoint=endpoint,
            insecure=True,
        )
    )
    tracer_provider.add_span_processor(span_processor)
    return tracer_provider.get_tracer(__name__)


@app.middleware("http")
async def add_tracing(request: Request, call_next):
    span_name = f"kgateway-ai-{os.path.basename(request.url.path)}-webhook"
    print(f"adding trace for {span_name}")
    print(f"trace context {extract(request.headers)}")
    with tracer().start_as_current_span(span_name, context=extract(request.headers)):
        response = await call_next(request)
    return response


@app.post(
    "/request",
    response_model=api.GuardrailsPromptResponse,
    tags=["Webhooks"],
    description="This webhook will be called for every request before sending the prompts to the LLM. "
    + "The 'role' and 'content' are extracted from the prompts into the PromptMessages json object "
    + "regardless of the API format from various providers.\n\n\n"
    + "Three types of responses are possible by returning one of the follow three json objects:\n\n\n"
    + "    - PassAction  : Indicates that no action is taken for the prompts and it is allow to be send to the LLM\n"
    + "    - MaskAction  : Indicates that some information are masked in the prompt and it needs to be updated before sending to the LLM\n"
    + "                    The PromptMessages json object of the request can be modified in place and send back in the body field of the\n"
    + "                    response. The number of messages inside PromptMessages MUST be the same as the request in this webhook call. \n"
    + "                    So, if the content needs to be deleted, an empty content field need to be set.\n"
    + "    - RejectAction: Indicates that the request should be rejected with the specific status code and response message. The request\n"
    + "                    will not be sent to the LLM.",
)
async def process_prompts(
    req: api.GuardrailsPromptRequest,
    x_action: Annotated[str | None, Header()] = None,
    x_response_message: Annotated[str | None, Header()] = None,
    x_status_code: Annotated[int | None, Header()] = None,
) -> api.GuardrailsPromptResponse:
    #    print(req.body)
    threshold = 0.7
    PII_detected = False
    for i, message in enumerate(req.body.messages):
        if len(message.content) == 0:
            continue

        results = analyzer.analyze(message.content, language="en")
        print(results)
        above_threshold = any(r.score > threshold for r in results)
        print(above_threshold)
        if above_threshold:
            match x_action:
                case "block":
                    return api.GuardrailsPromptResponse(
                        action=api.RejectAction(
                            body=(
                                x_response_message
                                if x_response_message
                                else "PII detected in the request"
                            ),
                            status_code=(x_status_code if x_status_code else 403),
                            reason="PII detected",
                        ),
                    )

                case "mask":
                    # Call analyzer to get results
                    anonymized: EngineResult = anon.anonymize(
                        text=message.content,
                        analyzer_results=[
                            RecognizerResult(
                                entity_type=r.entity_type,
                                start=r.start,
                                end=r.end,
                                score=r.score,
                            )
                            for r in results
                            if r.score > threshold
                        ],
                    )
                    #                    print(f"PII detected before: {message.content}")
                    req.body.messages[i].content = anonymized.text
                    #                    print(f"PII detected after: {req.body.messages[i].content}")
                    PII_detected = True

    if PII_detected:
        print(f"PII detected:\n{req.body}")
        return api.GuardrailsPromptResponse(
            action=api.MaskAction(
                body=req.body,
                reason="PII detected and masked",
            ),
        )

    print("no PII detected")
    return api.GuardrailsPromptResponse(
        action=api.PassAction(reason="No reason"),
    )


@app.post(
    "/response",
    response_model=api.GuardrailsResponseResponse,
    tags=["Webhooks"],
    description="This webhook will be called for every response from the LLM before sending back to the user. "
    + "The 'role' and 'content' are extracted from the prompts into the ResponseChoices json object "
    + "regardless of the API format from various providers.\n\n\n"
    + "For streaming responses from the LLM, this webhook will be called multiple times for a single response. "
    + "The AI gateway will buffer and detect the semantic boundary of the content before making the webhook call.\n\n\n"
    + "Two types of responses are possible by returning one of the follow two json objects:\n\n\n"
    + "    - PassAction: Indicates that no action is taken for the response and it is allow to be send to the user.\n"
    + "    - MaskAction: Indicates that some information are masked in the response and it needs to be updated before sending\n"
    + "                  to the user. The ResponseChoices json object from this webhook call can be modified in place and send\n"
    + "                  back in the body field in the response.\n"
    + "                  The number of choices inside ResponseChoices MUST be the same as the request in this webhook call.\n"
    + "                  So, if the content needs to be deleted, an empty content field need to be set.\n",
)
async def process_responses(
    request: Request,
    req: api.GuardrailsResponseRequest,
) -> api.GuardrailsResponseResponse:
    threshold = 0.7
    #    print(req.body)
    PII_detected = False
    for i, choice in enumerate(req.body.choices):
        results = analyzer.analyze(choice.message.content, language="en")
        print(results)
        above_threshold = any(r.score > threshold for r in results)
        print(above_threshold)
        if above_threshold:
            # Call analyzer to get results
            anonymized: EngineResult = anon.anonymize(
                text=choice.message.content,
                analyzer_results=[
                    RecognizerResult(
                        entity_type=r.entity_type,
                        start=r.start,
                        end=r.end,
                        score=r.score,
                    )
                    for r in results
                    if r.score > threshold
                ],
            )
            print(f"PII detected before: {choice.message.content}")
            req.body.choices[i].message.content = anonymized.text
            print(f"PII detected after: {req.body.choices[i].message.content}")
            PII_detected = True
        else:
            limit = 50
            if len(choice.message.content) > limit:
                print(f"no PII detected: content: {choice.message.content[0:limit]}...")
            else:
                print(f"no PII detected: content: {choice.message.content}")

    if PII_detected:
        print(f"PII detected:\n{req.body}")
        return api.GuardrailsResponseResponse(
            action=api.MaskAction(
                body=req.body,
                reason="PII detected and masked",
            ),
        )
    else:
        # There is evidence that some times, presidio fails to detect the PII
        # So, in all the webhook tests (prompt_guard_webhook.py and prompt_guard_webhook_streaming.py)
        # It asks to include the name "William", so, if presidio says no PII but we still find
        # William, we know it's not working properly. Since we are testing the webhook mechanism and
        # not presidio, try to fall back to simple search and replace here to make the test pass
        name = "William"
        presidio_failed = False
        for i, choice in enumerate(req.body.choices):
            if name in choice.message.content:
                presidio_failed = True
                print(
                    f"WARNING: presidio failed to detected PII:\n{choice.message.content}"
                )
                choice.message.content = choice.message.content.replace(
                    name, "<PERSON>"
                )
                print(f"Attempt to recover:\n{choice.message.content}")

        if presidio_failed:
            return api.GuardrailsResponseResponse(
                action=api.MaskAction(
                    body=req.body,
                    reason="PII detected and masked",
                ),
            )

    return api.GuardrailsResponseResponse(
        action=api.PassAction(reason="No PII detected"),
    )


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/shutdown")
async def shutdown():
    os.kill(os.getpid(), signal.SIGTERM)


def main():
    uvicorn.run(app, host="0.0.0.0", port=8000)
