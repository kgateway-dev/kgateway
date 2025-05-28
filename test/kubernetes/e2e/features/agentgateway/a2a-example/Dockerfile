FROM python:3.13-slim

RUN apt-get update && apt-get install -y git && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY . /app

RUN pip install .

EXPOSE 9090

ENTRYPOINT ["python", "main.py"]
