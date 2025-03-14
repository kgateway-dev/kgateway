import os
import sys

# Add the api directory to the path so that they can resolve each other
# without needing to use relative imports
sys.path.insert(
    0, os.path.join(os.path.dirname(os.path.realpath(__file__)), "..", "api")
)
