#!/usr/bin/env python3
import json
import time
import base64
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs
from jwcrypto import jwk, jwt

# ------------------------------
# Hardcoded credentials
# ------------------------------
HARDCODED_CLIENT_ID = "mcp_gi3APARn2_uHv2oxfJJqq2yZBDV4OyNo"
HARDCODED_CODE = "fixed_auth_code_123"
HARDCODED_CLIENT_SECRET = "secret_2nGx_bjvo9z72Aw3-hKTWMusEo2-yTfH"
HARDCODED_ACCESS_TOKEN = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjUzMzM3ODA2ODc1NTEwMzg2NTkifQ.eyJhdWQiOiJhY2NvdW50IiwiZXhwIjoxNzYzNjc2Nzc2LCJpYXQiOjE3NjM2NzMxNzYsImlzcyI6Imh0dHBzOi8va2dhdGV3YXkuZGV2Iiwic3ViIjoidXNlckBrZ2F0ZXdheS5kZXYifQ.Fko5TMFRRJoXyidRaAmzmwlVHIwNxCXqiKf5BRw_sumTnpNmt9Qt_2RUQCn7tTC_gAV50FyV4WKwoyTzAn0S8mmgZumI8E2-Uoq-A8wAohz9rt4a61_gaDeXXn0dF3YitQicR30Q_buoi2Nki6ZRPf9FyE5ulO4Ut_PyQrNXwlwO7vr_U3DXfrzvT9y2aDdNndPr1GB4fWTM84mEdQgx3XevIc7yjnbgKHnvIRp4gEyh-QL0ZYisjD-tZIDloZoSZjNFYu6PIdoxAaz9WhINAkAqX9KS8cd6uO36nPDoDOT1UmCT2VBjNszhLaZqtRKbJUb1HYrn-Gzq8vumLn8sjQ"
HARDCODED_REFRESH_TOKEN = "fixed_refresh_token_123"
REDIRECT_URI = "http:/localhost:8081/callback"

# Private JWK (for signing JWTs)
PRIVATE_JWK_JSON = """{
"d":"KEhvRCLz5YUve9YwaXQs8zJD6vvRygiaT60pkyADzAINVRIOsezVOUCX-aFzS14e5ioBKC9pYj74mPMoro1QlbmZMMbsps_9xO9iKSS3M83S7NckFOoZuJCmikRvQqahGbrokvUuEmS81ydMg6t5tCF918oBfL6A72DklnNgRxXDqv1ohY5h_z7z7eKjBXcxsF1S1Aybhbh4bOtiCgEb5Y5tlmIY_nDEsEw1oYAr2qj1Yib-Wbv5Lh05__kFvVM7DNKb6pq9PmZezMBzDP58BhMhsjt3rbdClE7gjD3ooAOGaMlRoZkP580SIC1hNVfrYibMBdH3GNze4q9cwR40KQ",
"dp":"5tQTAkVdUgFH4bxP6Nwit9b5mgdM112GwkUJnVoTctAADh7OPB3C18-6GqGE50-qv1AJWwEsP73t8CqrIyOFEs4iB7kKuucKZetnkbvjnbxFSHdSBrnlDkBkD4ajPXDx-KQ4HIjAnMvl0_sWDWrOd93G7aQFSHw2KbJnJMXx9d0",
"dq":"oDD86ei2GpPglWMvboNXHRdrtBNKDHRRLXI82ux1lvxD9wQbLBLhNLz3FgNFEiaKburNcExpPqRegEBifAiZvbHSOCb915MJS-x_MHob_TgHFA-IPIZvdFTSnFXBncPjIbT-J2ow5o3VR5_tE-kpmXfxl3GFcl6I4fUP-kxJjUU",
"e":"AQAB",
"kty":"RSA",
"n":"rmQv7yCC0-yR4zIirW1o04DrWRay04pXpugM7Kw7fdqiLIyFwxA6aQ_f1whyDvS3e-kGJnJ-byis9aNf9DJG_bYzdw99JFODlogY-AZL6449AeujbsaF_kxepZZiwkh9wLQ8zthr4ccVR7HM1AYlE___4ulNYwT5h5xuJOGZRsCb30I2AmDklFlPmtRGC5p7Pz5ZM0XBSfwRBPPa4mbWrghN_HhXrSDyj-eB1sp82clDvbI4hxet9_dW60jqTyj7xlh8jtl6yCviWNH_rqr8N3gvC1zFVYUJ6SbsY1LCHDWfXdqk8iYjtShTws4n5JX7VSHjqL1B5rdAo166eiqSGQ",
"p":"6hfY4tlo9rZ6roy1eF5T_YTYOCEU2gf0fsPkhF_KbdUoEgcn4CJZdqKsEgftz_dC5Ivfgy1cBhv3a377K5UW7RbDA1oGhTLPgwgdbhkc6b1_2bQFhb3mCY2EA7wET3OKenTrvX-xjccl3e1lsCoiJnduvVxJDQj7r4WMH-rrjeU",
"q":"vrYRgh8Eq5vrYG8vhWQd5RE-JBd8MBPbAnUUdmfFfs-neLq7rojUgMU7Z3WLQOwmcA1zW7zfvFp7yYxDoyx4gwvafV8FipAVM3SreWLddVWZ8VN_1c4s7Sv7tVp7jxhgfnROUI5NxHpvXYVCszC_dcKQVhpMKe2NkxGXmHIx0CU",
"qi":"PMk6mvu0hWMCD42vYg_gFG9jvztSMAS9O6JSw4d_ZAGPydhh0pTu38GrqVMFRH_7V389QpmgFJhYJ4QwqLH3zeTyG5OPB-zfoBJz2QSpLvGTTkgzr306pA57lhoMKN2adnF7HXBI2QRC1aInCqH40m8iQlTX-xR9LaWxasfhV9Q"
}"""

PRIVATE_KEY = jwk.JWK.from_json(PRIVATE_JWK_JSON)

# Public JWK (for JWKS endpoint)
PUBLIC_JWK_JSON = """{
"kty":"RSA",
"kid": "5333780687551038659",
"n":"rmQv7yCC0-yR4zIirW1o04DrWRay04pXpugM7Kw7fdqiLIyFwxA6aQ_f1whyDvS3e-kGJnJ-byis9aNf9DJG_bYzdw99JFODlogY-AZL6449AeujbsaF_kxepZZiwkh9wLQ8zthr4ccVR7HM1AYlE___4ulNYwT5h5xuJOGZRsCb30I2AmDklFlPmtRGC5p7Pz5ZM0XBSfwRBPPa4mbWrghN_HhXrSDyj-eB1sp82clDvbI4hxet9_dW60jqTyj7xlh8jtl6yCviWNH_rqr8N3gvC1zFVYUJ6SbsY1LCHDWfXdqk8iYjtShTws4n5JX7VSHjqL1B5rdAo166eiqSGQ",
"e":"AQAB"
}"""
PUBLIC_KEY = jwk.JWK.from_json(PUBLIC_JWK_JSON)

# Track registered clients and codes
registered_clients = {}
authorization_codes = {
    HARDCODED_CODE: {
        "client_id": HARDCODED_CLIENT_ID,
        "redirect_uri": REDIRECT_URI,
        "scope": "openid profile",
        "expires_at": time.time() + 600
    }
}

# ------------------------------
# HTTP Handler
# ------------------------------
class AuthServerHandler(BaseHTTPRequestHandler):

    def send_json_response(self, data, status_code=200):
        self.send_response(status_code)
        self.send_header('Content-Type', 'application/json')
        origin = self.headers.get('Origin', '*')
        # Echo specific Origin for credentialed requests; fallback to *
        self.send_header('Access-Control-Allow-Origin', origin)
        self.send_header('Vary', 'Origin')
        self.send_header('Access-Control-Allow-Credentials', 'true')
        # Mirror common request headers used by browsers for CORS flows
        request_headers = self.headers.get('Access-Control-Request-Headers', 'content-type, authorization')
        self.send_header('Access-Control-Allow-Headers', request_headers)
        self.end_headers()
        self.wfile.write(json.dumps(data).encode('utf-8'))

    def get_request_body(self):
        content_length = int(self.headers.get('Content-Length', 0))
        if content_length > 0:
            body = self.rfile.read(content_length).decode('utf-8')
            try:
                return dict(parse_qs(body))
            except:
                return {}
        return {}

    def do_POST(self):
        path = urlparse(self.path).path
        if path == '/register':
            self.handle_register()
        elif path == '/token':
            self.handle_token()
        else:
            self.send_json_response({"error": "not_found"}, 404)

    def do_GET(self):
        parsed_url = urlparse(self.path)
        path = parsed_url.path
        query_params = parse_qs(parsed_url.query)

        if path == '/authorize':
            self.handle_authorize(query_params)
        elif path == '/.well-known/jwks.json':
            self.handle_jwks()
        elif path == '/.well-known/oauth-authorization-server':
            self.handle_discovery()
        else:
            self.send_json_response({"error": "not_found"}, 404)

    def do_OPTIONS(self):
        origin = self.headers.get('Origin', '*')
        request_headers = self.headers.get('Access-Control-Request-Headers', 'content-type')
        self.send_response(204)
        self.send_header('Access-Control-Allow-Origin', origin)
        self.send_header('Vary', 'Origin')
        self.send_header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', request_headers)
        self.send_header('Access-Control-Allow-Credentials', 'true')
        self.end_headers()

    # ------------------------------
    # Endpoints
    # ------------------------------
    def handle_register(self):
        registration = {
            "client_id": HARDCODED_CLIENT_ID,
            "client_secret": HARDCODED_CLIENT_SECRET,
            "client_name": "Test Client",
            "client_description": "A test MCP client",
            "redirect_uris": [REDIRECT_URI],
            "grant_types": ["authorization_code", "refresh_token"],
            "response_types": ["code"],
            "token_endpoint_auth_method": "client_secret_basic",
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%S.%fZ"),
            "updated_at": time.strftime("%Y-%m-%dT%H:%M:%S.%fZ")
        }
        registered_clients[HARDCODED_CLIENT_ID] = registration
        self.send_json_response(registration)

    def handle_authorize(self, query_params):
        client_id = query_params.get('client_id', [''])[0]
        redirect_uri = query_params.get('redirect_uri', [''])[0]

        if client_id != HARDCODED_CLIENT_ID:
            self.send_json_response({"error": "invalid_client"}, 400)
            return

        callback_url = f"{redirect_uri}?code={HARDCODED_CODE}"
        self.send_json_response({"redirect_to": callback_url})

    def handle_token(self):
        body = self.get_request_body()
        grant_type = body.get('grant_type', [''])[0]

        # Extract Basic auth header if client_id not in body
        auth_header = self.headers.get('Authorization', '')
        client_id = body.get('client_id', [''])[0]
        client_secret = body.get('client_secret', [''])[0]

        if not client_id and auth_header.startswith('Basic '):
            decoded = base64.b64decode(auth_header.split(' ')[1]).decode('utf-8')
            client_id, client_secret = decoded.split(':', 1)

        if grant_type == 'authorization_code':
            # Be lenient for generic MCP inspectors/SPAs using PKCE:
            # - Do not require client_secret (public client)
            # - Accept any code/redirect_uri/code_verifier
            response = {
                "access_token": HARDCODED_ACCESS_TOKEN,
                "refresh_token": HARDCODED_REFRESH_TOKEN,
                "token_type": "bearer",
                "expires_in": 3600
            }
            self.send_json_response(response)

        elif grant_type == 'refresh_token':
            # For refresh token, still require confidential client auth
            if client_id != HARDCODED_CLIENT_ID or client_secret != HARDCODED_CLIENT_SECRET:
                self.send_json_response({"error": "invalid_client"}, 400)
                return
            refresh_token = body.get('refresh_token', [''])[0]
            # Accept any refresh_token for testing purposes

            access_token = self.issue_jwt(sub="user@kgateway.dev", aud="account")
            response = {
                "access_token": access_token,
                "refresh_token": HARDCODED_REFRESH_TOKEN,
                "token_type": "bearer",
                "expires_in": 3600
            }
            self.send_json_response(response)

        else:
            self.send_json_response({"error": "unsupported_grant_type"}, 400)

    def issue_jwt(self, sub, aud):
        payload = {
            "iss": "https://kgateway.dev",
            "sub": sub,
            "aud": aud,
            "iat": int(time.time()),
            "exp": int(time.time()) + 3600
        }
        token = jwt.JWT(header={"alg": "RS256"}, claims=payload)
        token.make_signed_token(PRIVATE_KEY)
        return token.serialize()

    def handle_jwks(self):
        jwks = {"keys": [json.loads(PUBLIC_JWK_JSON)]}
        self.send_json_response(jwks)

    # for local testing with port-forwarding gateway and mcp inspector
    def handle_discovery(self):
        discovery = {
            "issuer": "https://kgateway.dev",
            "authorization_endpoint": "http://localhost:8081/authorize",
            "token_endpoint": "http://localhost:8081/token",
            "jwks_uri": "http://localhost:8081/.well-known/jwks.json",
            "registration_endpoint": "http://localhost:8081/register",
            "response_types_supported": ["code"],
            "grant_types_supported": ["authorization_code", "refresh_token"],
            "token_endpoint_auth_methods_supported": ["none", "client_secret_basic", "client_secret_post"],
            "code_challenge_methods_supported": ["S256"]
        }
        self.send_json_response(discovery)

    def log_message(self, format, *args):
        print(f"[{self.address_string()}] {format % args}")


# ------------------------------
# Run server
# ------------------------------
def main():
    port = 9000
    server = HTTPServer(('0.0.0.0', port), AuthServerHandler)
    print(f"MCP Mock Auth Server running on http://0.0.0.0:{port}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down server...")
        server.shutdown()


if __name__ == '__main__':
    main()
