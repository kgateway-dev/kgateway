#!/usr/bin/env python3

import os
import logging

import httpserver.server

log_level = os.environ.get("LOG_LEVEL", "INFO").upper()
logging.basicConfig(level=log_level)
logger = logging.getLogger().getChild("kgateway-ai-ext")
logger.setLevel(log_level)


if __name__ == "__main__":
    httpserver.server.serve()
