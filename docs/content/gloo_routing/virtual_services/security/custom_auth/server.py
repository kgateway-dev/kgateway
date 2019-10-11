import http.server
import socketserver

class Server(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        path = self.path
        print("path", path)
        if path.startswith("/api/pets/1"):
            self.send_response(200, 'OK')
        else:
            self.send_response(401, 'Not authorized')
        self.send_header('x-server', 'pythonauth')
        self.end_headers()

def serve_forever(port):
    socketserver.TCPServer(('', port), Server).serve_forever()

if __name__ == "__main__":
    serve_forever(8000)
