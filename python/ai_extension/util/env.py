import os

pg_connection_str_env = os.getenv(
    "PG_CONNECTION_STR",
    "postgresql+psycopg://gloo:gloo@localhost:6024/gloo",
)
pg_collection_name_env = os.getenv("PG_COLLECTION_NAME", "default")

redis_url_env = os.getenv("REDIS_URL", "")

weaviate_url_env = os.getenv("WEAVIATE_URL", "")

open_ai_token_env = os.getenv("OPENAI_API_KEY", "")

azure_open_ai_token_env = os.getenv("AZURE_OPENAI_API_KEY", "")
