import base64
import json


def read_mounted_config_map(file_path: str) -> dict:
    with open(file_path, "rb") as file:
        # Read the file content as bytes
        base64_encoded_content = file.read()

    # Step 2: Base64 decode the content
    decoded_bytes = base64.b64decode(base64_encoded_content)

    # Step 3: Convert the decoded bytes to a UTF-8 string
    json_string = decoded_bytes.decode("utf-8")

    # Step 4: Parse the JSON content
    return json.loads(json_string)
