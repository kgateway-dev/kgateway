import os
import gzip

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

full_input_path = os.path.join(SCRIPT_DIR, 'data/routing/openai_non_streaming.json')
json_str = ""
with open(full_input_path, 'r') as fin:
    json_str = fin.read()

full_output_path = os.path.join(SCRIPT_DIR, 'data/routing/openai_non_streaming.txt.gz')
with gzip.open(full_output_path, 'wb') as fout:
    fout.write(json_str.encode('utf-8'))