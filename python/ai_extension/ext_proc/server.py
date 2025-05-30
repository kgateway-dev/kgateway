import os
import sys
import asyncio
import traceback
from contextlib import contextmanager
from copy import deepcopy

# Add the api directory to the path so that they can resolve each other
# without needing to use relative imports
sys.path.insert(
    0, os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "api")
)
from concurrent.futures import ThreadPoolExecutor
import logging
import json
from typing import AsyncIterable, Final
import signal
import grpc
import gzip

from telemetry.stats import Config as StatsConfig
from telemetry.tracing import Config as TracingConfig, OtelTracer
from .stream import Handler as StreamHandler
from guardrails.regex import RegexRejection

from openai import AsyncOpenAI as OpenAIClient
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from google.protobuf import struct_pb2 as struct_pb2
from prometheus_client import Counter, Histogram, start_http_server

from grpc_health.v1 import health
from grpc_health.v1 import health_pb2_grpc

from api.envoy.service.ext_proc.v3 import external_processor_pb2
from api.envoy.service.ext_proc.v3 import external_processor_pb2_grpc
from api.kgateway.policy.ai import prompt_guard
from util.proto import (
    extproc_clear_request_body,
    extproc_clear_response_body,
    extproc_new_response_body,
    get_auth_token,
    get_http_header,
    map_int_to_grpc_status_code,
)
from util.env import open_ai_token_env

from guardrails.api import (
    RejectAction,
    PromptMessages,
    ResponseChoices,
)

from guardrails.presidio import init_presidio_config
from guardrails.webhook import (
    make_request_webhook_request,
    make_response_webhook_request,
    WebhookException,
)
from presidio_analyzer import EntityRecognizer

# OpenTelemetry imports
from opentelemetry import trace

from opentelemetry.context import attach, detach
from opentelemetry.propagate import extract


# Listen address can be a Unix Domain Socket path or an address like [::]:18080
server_listen_addr = os.getenv("LISTEN_ADDR", "unix-abstract:kgateway-ai-sock")
unix_addr_prefix: Final[str] = "unix://"

logger = logging.getLogger().getChild("kgateway-ai-ext.external_processor")

llm_label_name: Final[str] = "llm"
model_label_name: Final[str] = "model"

ai_stat_namespace: Final[str] = "ai"


class ExtProcServer(external_processor_pb2_grpc.ExternalProcessorServicer):
    _prompt_tokens_ctr: Counter
    _completion_tokens_ctr: Counter
    _rate_limited_tokens_ctr: Counter
    _exception_raised: Counter
    _webhook_req_time_sec: Histogram
    _stats_config: StatsConfig

    def __init__(
        self,
        stats_config: StatsConfig,
        tracing_config: TracingConfig,
    ):
        self._req_guard: dict[str, list[EntityRecognizer]] = {}
        self._resp_guard: dict[str, list[EntityRecognizer]] = {}
        self._stats_config = stats_config

        labels = [llm_label_name, model_label_name]

        for custom_label in stats_config.custom_labels:
            labels.append(custom_label.name)

        self._prompt_tokens_ctr = Counter(
            "prompt_tokens", "Prompt tokens", labels, ai_stat_namespace
        )
        self._completion_tokens_ctr = Counter(
            "completion_tokens", "Completion tokens", labels, ai_stat_namespace
        )
        self._rate_limited_tokens_ctr = Counter(
            "rate_limited_tokens",
            "Tokens count toward rate limiting",
            labels,
            ai_stat_namespace,
        )

        OtelTracer.init(tracing_config.tracer())

        self._exception_raised = Counter(
            "exception_raised",
            "Exceptions raised when processing requests",
            labels,
            ai_stat_namespace,
        )

    @contextmanager
    def _set_remote_context(self, servicer_context):
        metadata = servicer_context.invocation_metadata()
        if metadata:
            md_dict = {md.key: md.value for md in metadata}
            ctx = extract(md_dict)
            token = attach(ctx)
            try:
                yield
            finally:
                detach(token)
        else:
            yield

    async def Process(
        self,
        request_iterator: AsyncIterable[external_processor_pb2.ProcessingRequest],
        context: grpc.ServicerContext,
    ) -> AsyncIterable[external_processor_pb2.ProcessingResponse]:
        handler = StreamHandler.from_metadata(dict(context.invocation_metadata()))
        handler.logger.debug(
            "Received a processing request %s", context.invocation_metadata()
        )

        with self._set_remote_context(context) as ctx:
            async for request in request_iterator:
                one_of = request.WhichOneof("request")
                handler.logger.info("one_of: %s", one_of)
                if one_of == "request_headers":
                    with OtelTracer.get().start_as_current_span(
                        "handle_request_headers",
                        context=ctx,
                    ) as header_span:
                        handler.logger.debug(
                            "Request headers:\n%s", request.request_headers
                        )
                        handler.build_extra_labels(
                            self._stats_config, request.metadata_context
                        )
                        with OtelTracer.get().start_as_current_span(
                            "parse_config",
                            context=trace.set_span_in_context(header_span),
                        ):
                            await self.parse_handler_config(
                                handler,
                                dict(context.invocation_metadata()),
                                request.request_headers,
                            )
                        try:
                            yield self.handle_request_headers(
                                request.request_headers, handler
                            )
                        except Exception as exc:
                            self.increment_exception_raised(handler)
                            header_span.record_exception(exc)
                            logger.error("Error handling request headers, %s", exc)
                            yield external_processor_pb2.ProcessingResponse(
                                request_headers=external_processor_pb2.HeadersResponse()
                            )
                elif one_of == "request_body":
                    with OtelTracer.get().start_as_current_span(
                        "handle_request_body",
                        context=ctx,
                    ) as parent_span:
                        handler.logger.debug("Request body:\n%s", request.request_body)
                        try:
                            yield await self.handle_request_body(
                                request.request_body,
                                dict(context.invocation_metadata()),
                                handler,
                                parent_span,
                            )
                        except Exception as exc:
                            self.increment_exception_raised(handler)
                            parent_span.record_exception(exc)
                            logger.error("Error handling request body, %s", exc)
                            logger.debug(traceback.format_exc())
                            yield external_processor_pb2.ProcessingResponse(
                                request_body=external_processor_pb2.BodyResponse()
                            )
                elif one_of == "request_trailers":
                    handler.logger.debug(
                        "Request trailers:\n%s", request.request_trailers
                    )
                    yield external_processor_pb2.ProcessingResponse(
                        request_trailers=external_processor_pb2.TrailersResponse()
                    )
                elif one_of == "response_headers":
                    with OtelTracer.get().start_as_current_span(
                        "handle_response_headers",
                        context=ctx,
                    ) as parent_span:
                        handler.logger.debug(
                            "Response headers:\n%s", request.response_headers
                        )
                        handler.content_encoding = get_http_header(
                            request.response_headers.headers, "content-encoding"
                        )
                        handler.resp.set_headers(
                            (
                                handler.resp_webhook.forwardHeaders
                                if handler.resp_webhook
                                else []
                            ),
                            request.response_headers,
                        )
                        # calling this after set_headers will avoid parsing the content-type header again
                        handler.resp.is_streaming = (
                            handler.provider.is_streaming_response(
                                is_streaming_request=handler.req.is_streaming,
                                response_headers=request.response_headers.headers,
                                content_type=handler.resp.content_type,
                            )
                        )
                        yield external_processor_pb2.ProcessingResponse(
                            response_headers=external_processor_pb2.HeadersResponse()
                        )
                elif one_of == "response_body":
                    with OtelTracer.get().start_as_current_span(
                        "handle_response_body",
                        context=ctx,
                    ) as parent_span:
                        handler.logger.debug(
                            "Response body:\n%s", request.response_body
                        )
                        try:
                            yield await self.handle_response_body(
                                request.response_body,
                                handler,
                                parent_span,
                            )
                        except WebhookException as webHookEx:
                            self.increment_exception_raised(handler)
                            parent_span.record_exception(webHookEx)
                            yield error_response(
                                prompt_guard.CustomResponse(status_code=500),
                                "Error with guardrails webhook",
                                webHookEx,
                            )
                        except Exception as exc:
                            self.increment_exception_raised(handler)
                            parent_span.record_exception(exc)
                            logger.error("Error handling response body, %s", exc)
                            logger.debug(traceback.format_exc())
                            yield external_processor_pb2.ProcessingResponse(
                                response_body=external_processor_pb2.BodyResponse()
                            )
                elif one_of == "response_trailers":
                    handler.logger.debug(
                        "Response trailers:\n%s", request.response_trailers
                    )
                    yield external_processor_pb2.ProcessingResponse(
                        response_trailers=external_processor_pb2.TrailersResponse()
                    )

    async def parse_handler_config(
        self,
        handler: StreamHandler,
        metadict: dict,
        headers: external_processor_pb2.HttpHeaders,
    ):
        if (guardrails := metadict.get("x-req-guardrails-config", "")) != "":
            guardrails_obj = prompt_guard.req_from_json(guardrails)
            config_hash = metadict.get("x-req-guardrails-config-hash", "")
            if guardrails_obj.custom_response:
                handler.req_custom_response = guardrails_obj.custom_response
            if guardrails_obj.moderation:
                if guardrails_obj.moderation.openai:
                    handler.req_moderation = (
                        OpenAIClient(
                            api_key=get_auth_token(
                                guardrails_obj.moderation.openai.auth_token,
                                headers.headers,
                                open_ai_token_env,
                            )
                        ).moderations,
                        guardrails_obj.moderation.openai.model,
                    )
                else:
                    raise ValueError("Unknown moderation type")
            if guardrails_obj.webhook:
                handler.req_webhook = guardrails_obj.webhook
            if guardrails_obj.regex:
                handler.req_regex_action = guardrails_obj.regex.action
                if config_hash in self._req_guard:
                    handler.req_regex = self._req_guard.get(config_hash)
                    logger.debug("reusing cached request regex")
                else:
                    recognizers = init_presidio_config(guardrails_obj.regex)
                    handler.req_regex = recognizers
                    if handler.req_regex is not None:
                        self._req_guard[config_hash] = handler.req_regex

        if (guardrails := metadict.get("x-resp-guardrails-config", "")) != "":
            guardrails_obj = prompt_guard.resp_from_json(guardrails)
            config_hash = metadict.get("x-resp-guardrails-config-hash", "")
            if guardrails_obj.webhook:
                handler.resp_webhook = guardrails_obj.webhook
            if guardrails_obj.regex:
                if config_hash in self._resp_guard:
                    handler.resp_regex = self._resp_guard.get(config_hash)
                    logger.debug("reusing cached response regex")
                else:
                    recognizers = init_presidio_config(guardrails_obj.regex)
                    handler.resp_regex = recognizers
                    if handler.resp_regex is not None:
                        self._resp_guard[config_hash] = handler.resp_regex

        return handler

    def handle_request_headers(
        self, headers: external_processor_pb2.HttpHeaders, handler: StreamHandler
    ) -> external_processor_pb2.ProcessingResponse:
        handler.req.set_headers(
            handler.req_webhook.forwardHeaders if handler.req_webhook else [],
            headers,
        )
        auth_header = get_http_header(headers.headers, "authorization").removeprefix(
            "Bearer "
        )

        return external_processor_pb2.ProcessingResponse(
            dynamic_metadata=struct_pb2.Struct(
                fields={
                    "ai.kgateway.io": struct_pb2.Value(
                        struct_value=struct_pb2.Struct(
                            fields={
                                "auth_token": struct_pb2.Value(
                                    string_value=auth_header
                                ),
                            }
                        )
                    )
                },
            ),
            request_headers=external_processor_pb2.HeadersResponse(),
        )

    async def handle_request_body_req_webhook(
        self,
        body: dict,
        handler: StreamHandler,
        webhook_cfg: prompt_guard.Webhook,
        parent_span: trace.Span,
    ) -> external_processor_pb2.ProcessingResponse | None:
        with OtelTracer.get().start_as_current_span(
            "webhook",
            context=trace.set_span_in_context(parent_span),
        ):
            try:
                headers = deepcopy(handler.req.headers)
                TraceContextTextMapPropagator().inject(headers)
                response: (
                    PromptMessages | RejectAction | None
                ) = await make_request_webhook_request(
                    webhook_host=webhook_cfg.host,
                    webhook_port=webhook_cfg.port,
                    headers=headers,
                    promptMessages=handler.provider.construct_request_webhook_request_body(
                        body
                    ),
                )

                if isinstance(response, PromptMessages):
                    handler.provider.update_request_body_from_webhook(body, response)

                if isinstance(response, RejectAction):
                    return external_processor_pb2.ProcessingResponse(
                        immediate_response=external_processor_pb2.ImmediateResponse(
                            status=dict(
                                code=map_int_to_grpc_status_code(response.status_code)
                            ),
                            body=(
                                response.body
                                if response.body != ""
                                else "Rejected by guardrails webhook"
                            ).encode("utf-8"),
                            details=response.reason,
                        ),
                    )

                # When response is None, that means webhook did not modified anything,
                # so just proceed (return None)
            except Exception as e:
                return error_response(
                    handler.req_custom_response, "Error with guardrails webhook", e
                )

    def handle_request_body_req_regex(
        self, body: dict, handler: StreamHandler, parent_span: trace.Span
    ) -> external_processor_pb2.ProcessingResponse | None:
        with OtelTracer.get().start_as_current_span(
            "regex",
            context=trace.set_span_in_context(parent_span),
        ):
            # If this raises an exception it means that the action was reject, not mask
            try:
                handler.provider.iterate_str_req_messages(
                    body=body, cb=handler.req_regex_transform
                )
            except RegexRejection as e:
                return error_response(
                    handler.req_custom_response, "Rejected by guardrails regex", e
                )

    async def handle_request_body(
        self,
        req_body: external_processor_pb2.HttpBody,
        metadict: dict,
        handler: StreamHandler,
        parent_span: trace.Span,
    ) -> external_processor_pb2.ProcessingResponse:
        # Always append the request body
        handler.req.append(req_body.body)
        if req_body.end_of_stream:
            body_jsn = json.loads(handler.req.body.decode("utf-8"))
            handler.request_model = handler.provider.get_model_req(body_jsn, metadict)

            # Check if request is streaming
            # TODO(npolshak): Remove x-chat-streaming header once have access to request path
            handler.req.is_streaming = handler.provider.is_streaming_req(
                body_jsn, metadict
            )

            body = body_jsn
            with OtelTracer.get().start_as_current_span(
                "count_tokens",
                context=trace.set_span_in_context(parent_span),
            ):
                tokens = handler.provider.get_num_tokens_from_body(body)

            if handler.req_webhook and (
                req_webhook := await self.handle_request_body_req_webhook(
                    body, handler, handler.req_webhook, parent_span
                )
            ):
                return req_webhook

            if handler.req_regex and (
                req_regex_resp := self.handle_request_body_req_regex(
                    body, handler, parent_span=parent_span
                )
            ):
                return req_regex_resp

            if handler.req_moderation:
                with OtelTracer.get().start_as_current_span(
                    "moderation",
                    context=trace.set_span_in_context(parent_span),
                ):
                    (client, model) = handler.req_moderation
                    results = await client.create(
                        input=handler.provider.all_req_content(body),
                        model=(model if model != "" else "omni-moderation-latest"),
                    )
                    for result in results.results:
                        if result.flagged:
                            return error_response(
                                handler.req_custom_response,
                                "Rejected by guardrails moderation",
                            )

            # currently we only count the prompt token for ratelimiting. So,
            # this is only set here. If we change to count completion token as well
            # will need to add those into rate_limited_tokens for stats purpose.
            handler.rate_limited_tokens = tokens
            return external_processor_pb2.ProcessingResponse(
                dynamic_metadata=struct_pb2.Struct(
                    # increment tokens for rate limiting
                    fields={
                        "envoy.ratelimit": struct_pb2.Value(
                            struct_value=struct_pb2.Struct(
                                fields={
                                    "hits_addend": struct_pb2.Value(
                                        number_value=float(tokens),
                                    )
                                }
                            )
                        )
                    },
                ),
                request_body=external_processor_pb2.BodyResponse(
                    response=external_processor_pb2.CommonResponse(
                        body_mutation=external_processor_pb2.BodyMutation(
                            body=json.dumps(body).encode("utf-8")
                        ),
                    ),
                ),
            )

        # If it's not end of stream, clear the body so envoy doesn't forward to upstream.
        return extproc_clear_request_body()

    async def handle_response_body(
        self,
        resp_body: external_processor_pb2.HttpBody,
        handler: StreamHandler,
        parent_span: trace.Span,
    ) -> external_processor_pb2.ProcessingResponse:
        if handler.resp.is_streaming:
            # TODO(npolshak): Prompt guard is only applied to the last function call response.
            # We can optimize and avoid buffering the intermediate response
            body = await handler.stream_chunks.buffer(
                llm_provider=handler.provider,
                resp_webhook=handler.resp_webhook,
                resp_regex=handler.resp_regex,
                anonymizer_engine=handler.anon,
                resp_headers=handler.resp.headers,
                resp_body=resp_body,
                parent_span=parent_span,
            )
            if body is None:
                handler.logger.debug(
                    "buffering streaming response %s\n", resp_body.body
                )
                return extproc_clear_response_body()

            handler.resp.append(body)
            handler.logger.debug("handling streaming response %s\n", body)
            if resp_body.body == body:
                extproc_response_body = external_processor_pb2.BodyResponse()
            else:
                extproc_response_body = external_processor_pb2.BodyResponse(
                    response=external_processor_pb2.CommonResponse(
                        body_mutation=external_processor_pb2.BodyMutation(body=body),
                    ),
                )

            if resp_body.end_of_stream:
                return external_processor_pb2.ProcessingResponse(
                    response_body=extproc_response_body,
                    dynamic_metadata=self.build_dynamic_meta(handler),
                )

            return external_processor_pb2.ProcessingResponse(
                response_body=extproc_response_body
            )

        else:
            handler.logger.debug("handling non streaming response %s\n", resp_body.body)
            # Keep track of the response body both for non-streaming responses.
            handler.resp.append(resp_body.body)

            if not resp_body.end_of_stream:
                # if it's not end of stream, tell envoy to clear the body
                # so we buffer the entire response first for GuardRail
                # TODO(andy): as an optimization, we can check if GuardRail is not enabled,
                #             we could tell envoy to just start returning the chunk here while
                #             we still store them for caching
                return extproc_clear_response_body()
            else:
                full_body = b""
                try:
                    full_body = (
                        gzip.decompress(handler.resp.body)
                        if handler.content_encoding == "gzip"
                        else bytes(handler.resp.body)
                    )
                    if handler.content_encoding == "gzip":
                        handler.logger.debug(f"unzipped body: {full_body}")

                    body_str = full_body.decode("utf-8")
                    jsn = json.loads(body_str)
                except json.decoder.JSONDecodeError as exc:
                    # This could be we are getting an error response that's not json in the body
                    handler.logger.debug("Error decoding json: %s", exc)
                    if full_body == resp_body.body:
                        # This means that there is only 1 chunk and it's also the end of stream,
                        # so, just ask envoy to send the response through
                        return external_processor_pb2.ProcessingResponse(
                            response_body=external_processor_pb2.BodyResponse()
                        )
                    else:
                        # This means we have already buffered some of the body, we should
                        # send them all back to envoy
                        return extproc_new_response_body(
                            content_encoding=handler.content_encoding, body=full_body
                        )
                else:
                    handler.increment_tokens(jsn)
                    has_function_call_resp = (
                        handler.provider.has_function_call_finish_reason(jsn)
                    )
                    handler.set_is_function_calling_response(has_function_call_resp)
                    handler.set_response_model(handler.provider.get_model_resp(jsn))

                    if handler.resp_webhook and not has_function_call_resp:
                        with OtelTracer.get().start_as_current_span(
                            "webhook",
                            context=trace.set_span_in_context(parent_span),
                        ):
                            try:
                                response: (
                                    ResponseChoices | None
                                ) = await make_response_webhook_request(
                                    webhook_host=handler.resp_webhook.host,
                                    webhook_port=handler.resp_webhook.port,
                                    headers=handler.resp.headers,
                                    rc=handler.provider.construct_response_webhook_request_body(
                                        body=jsn
                                    ),
                                )

                                if response is not None:
                                    handler.provider.update_response_body_from_webhook(
                                        jsn, response
                                    )
                            except Exception as e:
                                # This indicate the response webhook call failed (not from RejectAction)
                                # and we might have already sent out the response status code already to
                                # the end user. Returning an error here will cause Envoy to close the client
                                # connection immediately.
                                return error_response(
                                    prompt_guard.CustomResponse(status_code=500),
                                    "Error with guardrails webhook",
                                    e,
                                )

                    # Only run regex if the response has no tools
                    if handler.resp_regex and not has_function_call_resp:
                        with OtelTracer.get().start_as_current_span(
                            "regex",
                            context=trace.set_span_in_context(parent_span),
                        ):
                            handler.provider.iterate_str_resp_messages(
                                body=jsn, cb=handler.resp_regex_transform
                            )

                    return external_processor_pb2.ProcessingResponse(
                        response_body=external_processor_pb2.BodyResponse(
                            response=external_processor_pb2.CommonResponse(
                                body_mutation=external_processor_pb2.BodyMutation(
                                    body=(
                                        gzip.compress(json.dumps(jsn).encode("utf-8"))
                                        if handler.content_encoding == "gzip"
                                        else json.dumps(jsn).encode("utf-8")
                                    ),
                                ),
                            )
                        ),
                        dynamic_metadata=self.build_dynamic_meta(handler),
                    )

    def build_dynamic_meta(self, handler: StreamHandler) -> struct_pb2.Struct:
        labels = handler.extra_labels.copy()
        labels[llm_label_name] = handler.llm_provider
        labels[model_label_name] = handler.request_model

        tokens = handler.get_tokens()
        increment_counter(self._completion_tokens_ctr, labels, tokens.completion)
        increment_counter(self._prompt_tokens_ctr, labels, tokens.prompt)
        increment_counter(
            self._rate_limited_tokens_ctr, labels, handler.rate_limited_tokens
        )

        return handler.build_metadata()

    def increment_exception_raised(self, handler: StreamHandler):
        labels = handler.extra_labels.copy()
        labels[llm_label_name] = handler.llm_provider
        labels[model_label_name] = (
            handler.request_model
            if handler.request_model
            else handler.get_response_model()
        )
        increment_counter(
            self._exception_raised,
            labels,
            1,
        )


# Function to increment a counter and log any errors rather than
# stopping the request/response logic flow.
def increment_counter(counter: Counter, labels: dict[str, str], value: int):
    try:
        counter.labels(**labels).inc(value)
    except ValueError as e:
        logger.error(f"Error incrementing counter: {e}, continuing")


def error_response(
    custom_response: prompt_guard.CustomResponse | None,
    message: str,
    e: Exception | None = None,
):
    return external_processor_pb2.ProcessingResponse(
        immediate_response=external_processor_pb2.ImmediateResponse(
            status=dict(
                code=map_int_to_grpc_status_code(
                    custom_response.status_code if custom_response else 200
                )
            ),
            body=(
                custom_response.message.encode("utf-8")
                if custom_response
                else message.encode("utf-8")
            ),
            details=f"{message}: {e}",
        ),
    )


async def serve() -> None:
    if is_unix_socket(server_listen_addr):
        sock_path = server_listen_addr[len(unix_addr_prefix) :]
        if os.path.exists(sock_path):
            os.unlink(sock_path)

    stats_config = StatsConfig.from_file(file_path="/var/run/stats/stats.json")
    tracing_config = TracingConfig.from_file(file_path="/var/run/stats/tracing.json")

    address = server_listen_addr

    health_servicer = health.HealthServicer(
        experimental_non_blocking=True,
        experimental_thread_pool=ThreadPoolExecutor(),
    )
    # Start ExtProc gRPC server
    server = grpc.aio.server(
        ThreadPoolExecutor(),
    )
    external_processor_pb2_grpc.add_ExternalProcessorServicer_to_server(
        ExtProcServer(stats_config, tracing_config), server
    )
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    server.add_insecure_port(address)
    await server.start()
    logger.info("Server serving at %s", address)

    # Start prometheus server
    start_http_server(9092)
    logger.info("metrics serving on :9092/metrics")

    async def handle_sigterm(server: grpc.aio.Server):
        """Handle graceful shutdown on SIGTERM."""
        print("Received SIGTERM. Shutting down gracefully...")
        await server.stop(5)  # Stop with a graceful period (5 seconds)
        print("Server shut down completed.")

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(
            sig, lambda: asyncio.create_task(handle_sigterm(server))
        )

    print("Server started. Waiting for termination signal...")
    await server.wait_for_termination()


def is_unix_socket(address: str) -> bool:
    return address.startswith(unix_addr_prefix)
