from openai import OpenAI
from tenacity import retry


@retry()
def completion_req(client: OpenAI, **kwargs):
    resp = client.chat.completions.with_raw_response.create(**kwargs)
    completion = resp.parse()
    return completion, resp.headers
