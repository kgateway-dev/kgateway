from flask import Flask, jsonify, Response, request
from flask_cors import CORS
import json
import hashlib
import gzip
import os
import time
app = Flask(__name__)
CORS(app)  # Allows all origins

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

def get_json_hash(data, provider, stream=False):
    data["provider"] = provider # Dedup providers
    data["stream"] = stream # Dedup stream
    json_string = json.dumps(data, sort_keys=True) 
    return hashlib.sha256(json_string.encode()).hexdigest()

test_data = {
    # non streaming responses
    'b3393f0115ce2e97765fa0b4357494ac6580560a5a68f2431d55002ea8f3fe31': 'data/routing/azure_non_streaming.json',
    'fdcaa093f659f4035e1502c2d7b4ed8160365330513b20ec1deed795327037b3': 'data/routing/openai_non_streaming.txt.gz',
    'e2024a7184c0c592d8a816a339bb3e738bab2812ff57b84a1f37dfd366e395c8': 'data/routing/gemini_non_streaming.json',
    '3318c38b73f980b4382a40cfcd2eed1b00092cda9cb5335c8d76ff0db673101e': 'data/routing/vertex_ai_non_streaming.json',
    # streaming responses
    '8c0c43778f32b881436ca410b66bd134a9acfabbf7bf800d9e05ad47aebb6696': 'data/streaming/azure_streaming.txt',
    '705bf37e4ef6d83df189e431aeb6515ac101cce05bbd0056d8aa33da140c724b': 'data/streaming/openai_streaming.txt',
    '0c9bd70d83dc1a95cb363a41740e3ae08bc7dc8093c70dbcde23c7f4683ec10c': 'data/streaming/gemini_streaming.txt',
    '0ece1636671339ae8565d76c3150ae4a58dd1842b2fe53da90af125b95f711f0': 'data/streaming/vertex_ai_streaming.txt',
}

@app.route('/')
def index():
    response = jsonify({
        'status': 'healthy',
        'service': 'mock-provider',
    })
    response.status_code = 200  
    return response

def generate_sse_stream(file_path):
    with open(file_path, "r") as file:
        while chunk := file.readline(): 
            event_data = f"{chunk.strip()}\n\n"
            yield event_data
            time.sleep(0.1) # add a delay between chunks

def handle_model_response(data, provider, stream=False):
    hash = get_json_hash(data, provider, stream)
    print(f"data: {data}, hash: {hash}\n")
    responseFile = test_data.get(hash)
    if responseFile:
        response_content = None
        full_path = os.path.join(SCRIPT_DIR, responseFile)
        if stream:
            return Response(generate_sse_stream(full_path), mimetype='text/event-stream')
        else:
            is_gzipped = responseFile.endswith('.txt.gz')
            if is_gzipped:
                # handle gzip response
                with gzip.open(full_path, 'rb') as file:
                    response_content = file.read().decode("utf-8")
            else:
                with open(full_path, 'r') as file:
                    response_content = file.read()
            
            response = Response(response_content, mimetype='application/json')
            if is_gzipped:
                response.headers['Content-Encoding'] = 'gzip'
            return response
    else:
        return Response({"message": "Mock response not found"}, mimetype='application/json'), 404


@app.route('/v1/chat/completions', methods=['POST'])
def mock_openai_response():
    data = request.get_json()
    return handle_model_response(data, 'openai', stream=data.get('stream', False))

# ?api-version=2024-02-15-preview
@app.route('/openai/deployments/gpt-4o-mini/chat/completions', methods=['POST'])
def mock_azure_openai_response():
    data = request.get_json()
    return handle_model_response(data, 'azure', stream=data.get('stream', False))

@app.route('/v1beta/models/gemini-1.5-flash:generateContent', methods=['POST'])
def mock_gemini_response():
    return handle_model_response(request.get_json(), 'gemini')

@app.route('/v1beta/models/gemini-1.5-flash:streamGenerateContent', methods=['POST'])
def mock_gemini_streaming_response():
    return handle_model_response(request.get_json(), 'gemini', stream=True)

@app.route('/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:generateContent', methods=['POST'])
def mock_vertex_ai_response():
    return handle_model_response(request.get_json(), 'vertex_ai')

@app.route('/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:streamGenerateContent', methods=['POST'])
def mock_vertex_ai_streaming_response():
    # Use adhoc ssl context to avoid certificate errors
    return handle_model_response(request.get_json(), 'vertex_ai', ssl_context='adhoc', stream=True)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5001, debug=True, ssl_context='adhoc')
