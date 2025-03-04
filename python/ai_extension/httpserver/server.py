import logging
import threading
import asyncio
import os
import sys
import uvicorn
import signal
import uvicorn.config

from ext_proc.stream import (
    Provider,
)

sys.path.insert(
    0, os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "api")
)

from fastapi import APIRouter, HTTPException
from fastapi.responses import PlainTextResponse
from fastapi import FastAPI

from kubernetes import config, dynamic
from kubernetes.client import api_client

log_level = os.environ.get("LOG_LEVEL", "INFO").upper()
logging.basicConfig(level=log_level)
logger = logging.getLogger().getChild("kgateway-ai-apiserver")
logger.setLevel(log_level)


class APIServer:
    dynamic_client: dynamic.DynamicClient
    router: APIRouter

    def __init__(self, kube_client: dynamic.DynamicClient):
        self.dynamic_client = kube_client
        self.router = APIRouter()

        self.router.add_api_route(
            "/health",
            self.health,
            methods=["GET"],
            summary="Health check",
            description="Check if the server is running",
        )

    async def validate_request_parameters(
        self,
        llm_provider: Provider,
        model: str | None,
        stream: bool | None,
        req_js: dict,
    ) -> tuple[str, bool]:
        # Check if the model is provided; if not, check in the request body
        if model is None or model == "":
            model = llm_provider.get_model_req(req_js, {})

        # If model cannot be found, raise an error
        if model is None:
            raise HTTPException(
                status_code=400,
                detail="'model' must be provided in the request body or headers.",
            )

        # Check if the request is a streaming request; if not, check in the request body
        if stream is None:
            stream = llm_provider.is_streaming_req(req_js, {})

        # If stream cannot be found, raise an error
        if stream is None:
            raise HTTPException(
                status_code=400,
                detail="'stream' must be provided in the request body or headers.",
            )

        return model, stream

    async def health(self):
        return PlainTextResponse("OK")


class WebServer:
    def __init__(self, config: uvicorn.Config):
        self.server = uvicorn.Server(config)
        self.thread = threading.Thread(daemon=True, target=self.server.run)

    def start(self):
        self.thread.start()
        asyncio.run(self.wait_for_started())

    async def wait_for_started(self):
        while not self.server.started:
            await asyncio.sleep(0.1)

    def stop(self):
        if self.thread.is_alive():
            self.server.should_exit = True
            while self.thread.is_alive():
                continue


def serve() -> None:
    # Creating a dynamic client
    try:
        kube_config = config.load_incluster_config()
    except Exception as e:
        logger.error(f"Error loading in-cluster config: {e}, falling back to local")
        kube_config = config.load_kube_config()
    client = dynamic.DynamicClient(api_client.ApiClient(configuration=kube_config))

    # Start API server
    api_app = FastAPI()
    api_service = APIServer(client)
    api_app.include_router(api_service.router)
    uvicorn.config.LOGGING_CONFIG
    web_server = WebServer(
        uvicorn.Config(api_app, host="0.0.0.0", port=8000, log_config=None)
    )
    web_server.start()

    done = threading.Event()

    def on_done(signum, frame):
        logger.info("Got signal {}, {}".format(signum, frame))
        done.set()

    signal.signal(signal.SIGTERM, on_done)
    done.wait()
