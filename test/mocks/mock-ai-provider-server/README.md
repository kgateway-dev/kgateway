## Set-up Python virtualenv

```bash
python3 -m venv .venv
source .venv/bin/activate

python3 -m ensurepip --upgrade
python3 -m pip install -r requirements.txt

# set the PYTHON environment variable, required by the tests
export PYTHON=$(which python)
```

## Mocking gzip responses

Some providers (such as OpenAI) may respond with gzip-compressed data, which needs to be properly handled when mocking responses. To ensure compatibility, the mock server should detect when gzip encoding is requested and return appropriately compressed responses.

When returning a response in gzip format, you need to:

1. Compress the JSON response. You can use the example in `convert_to_gzip.py` as a template
2. Set the Content-Encoding header to "gzip" so clients can decode it properly.

## Mocking streaming responses 

Streaming responses are stored in Server-Sent Events (SSE) format and sent back in chunks. 

To mock SSE streaming responses in your server, you need to:
1. Set the correct response headers for streaming (`text/event-stream`)
2. Use a generator to send data in chunks.

## Mocking Requests

```shell
# gemini
curl localhost:5001/v1beta/models/gemini-1.5-flash:generateContent -v \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}],"generationConfig":{}}'

# streaming 
curl localhost:5001/v1beta/models/gemini-1.5-flash:streamGenerateContent -v \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}],"generationConfig":{}}'

# raw request

```

```shell
# vertex ai
# /v1/projects/kgateway-project/locations/us/publishers/google/models/gemini-1.5-flash-001:generateContent
# /v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:generateContent
curl localhost:5001/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:generateContent -v \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}]}'

# streaming
curl localhost:5001/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:streamGenerateContent -v \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"parts":[{"text":"Compose a poem that explains the concept of recursion in programming."}],"role":"user"}]}'
```

```shell
# azure 
curl http://localhost:5001/openai/deployments/gpt-4o-mini/chat/completions?api-version=2024-02-15-preview \
  -v -H "Content-Type: application/json" \
  -d '{
        "messages": [
          {
            "role": "system", 
            "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
          },
          {
            "role": "user", 
            "content": "Compose a poem that explains the concept of recursion in programming."
          }
        ], 
        "model": "gpt-4o-mini"
      }'

# streaming 
curl http://localhost:5001/openai/deployments/gpt-4o-mini/chat/completions?api-version=2024-02-15-preview \
  -v -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {
        "role": "system",
        "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
      },
      {
        "role": "user",
        "content": "Compose a poem that explains the concept of recursion in programming."
      }
    ],
    "model": "gpt-4o-mini",
    "stream": true
  }'

```

```shell
# openai
curl http://localhost:5001/v1/chat/completions -v -H "Content-Type: application/json" -d '{
  "messages": [
    {
      "role": "system",
      "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
    },
    {
      "role": "user",
      "content": "Compose a poem that explains the concept of recursion in programming."
    }
  ],
  "model": "gpt-4o-mini"
}'

# streaming
curl http://localhost:5001/v1/chat/completions -v -H "Content-Type: application/json" -d '{
  "messages": [
    {
      "role": "system",
      "content": "You are a poetic assistant, skilled in explaining complex programming concepts with creative flair."
    },
    {
      "role": "user",
      "content": "Compose a poem that explains the concept of recursion in programming."
    }
  ],
  "model": "gpt-4o-mini",
  "stream": true
}'

```