# using ubuntu LTS version
FROM python:3.12-slim-bookworm AS builder-image

# avoid stuck build due to user prompt
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install --no-install-recommends -y build-essential && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

# create and activate virtual environment
RUN python3 -m venv /opt/venv
ENV VIRTUAL_ENV=/opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# pass in the AI extension directory
ARG PYTHON_DIR

# install requirements
COPY $PYTHON_DIR/requirements.txt .
RUN pip3 install --no-cache-dir wheel
RUN pip3 install --no-cache-dir -r requirements.txt
# Pre-download tiktoken encoding model
ENV TIKTOKEN_CACHE_DIR="/opt/tiktoken"
RUN python3 -c "import tiktoken; tiktoken.get_encoding('cl100k_base')"

ENV TLDEXTRACT_CACHE="/opt/tldextract"
RUN python3 -m tldextract --update

FROM python:3.12-slim-bookworm AS runner-image

RUN apt-get update && apt-get install --no-install-recommends -y build-essential && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# pass in the AI extension directory for this step
ARG PYTHON_DIR

RUN useradd --create-home kgateway
COPY --from=builder-image /opt/venv /opt/venv
COPY --from=builder-image /opt/tldextract /opt/tldextract
COPY --from=builder-image /opt/tiktoken /opt/tiktoken

RUN chown -R kgateway:kgateway /opt/tldextract
RUN chmod 755 /opt/tldextract
RUN chown -R kgateway:kgateway /opt/tiktoken
RUN chmod 755 /opt/tiktoken

USER kgateway
RUN mkdir -p /home/kgateway/python
WORKDIR /home/kgateway
COPY $PYTHON_DIR/ai_extension /home/kgateway/python

# make sure all messages always reach console and
# avoid creation of .pyc files and activate virtual environment
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    VIRTUAL_ENV=/opt/venv \
    PATH="/opt/venv/bin:$PATH" \
    TLDEXTRACT_CACHE="/opt/tldextract" \
    TIKTOKEN_CACHE_DIR="/opt/tiktoken"

CMD ["python3", "python/main.py"]
