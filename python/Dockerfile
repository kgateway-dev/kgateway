# using ubuntu LTS version
FROM python:3.12-slim-bookworm AS builder-image

# avoid stuck build due to user prompt
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install --no-install-recommends -y build-essential libpq-dev && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

# create and activate virtual environment
RUN python3 -m venv /opt/venv
ENV VIRTUAL_ENV=/opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# install requirements
COPY requirements.txt .
RUN pip3 install --no-cache-dir wheel
RUN pip3 install --no-cache-dir -r requirements.txt
# Pre-download tiktoken encoding model
ENV TIKTOKEN_CACHE_DIR="/opt/tiktoken"
RUN python3 -c "import tiktoken; tiktoken.get_encoding('cl100k_base')"

ENV TLDEXTRACT_CACHE="/opt/tldextract"
RUN python3 -m tldextract --update

FROM python:3.12-slim-bookworm AS runner-image

RUN apt-get update && apt-get install --no-install-recommends -y build-essential libpq-dev && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home gloo
COPY --from=builder-image /opt/venv /opt/venv
COPY --from=builder-image /opt/tldextract /opt/tldextract
COPY --from=builder-image /opt/tiktoken /opt/tiktoken

RUN chown -R gloo:gloo /opt/tldextract
RUN chmod 755 /opt/tldextract
RUN chown -R gloo:gloo /opt/tiktoken
RUN chmod 755 /opt/tiktoken

USER gloo
RUN mkdir -p /home/gloo
WORKDIR /home/gloo
COPY ai_extension .

# make sure all messages always reach console
ENV PYTHONUNBUFFERED=1
# avoid creation of .pyc files
ENV PYTHONDONTWRITEBYTECODE=1

# activate virtual environment
ENV VIRTUAL_ENV=/opt/venv
ENV PATH="/opt/venv/bin:$PATH"
ENV TLDEXTRACT_CACHE="/opt/tldextract"
ENV TIKTOKEN_CACHE_DIR="/opt/tiktoken"

CMD ["python3", "main.py"]