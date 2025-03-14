#!/usr/bin/env python3

import os
import logging
import asyncio

import ext_proc.server

log_level = os.environ.get("LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=log_level,
    format="%(asctime)s - %(levelname)s - %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger().getChild("kgateway-ai-ext")
logger.setLevel(log_level)


if __name__ == "__main__":
    asyncio.run(ext_proc.server.serve())
