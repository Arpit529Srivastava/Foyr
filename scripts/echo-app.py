#!/usr/bin/env python3
"""A throwaway app for trying login locally: it prints the request it received, headers included."""
import os
from http.server import BaseHTTPRequestHandler, HTTPServer


class Echo(BaseHTTPRequestHandler):
    def do_GET(self):
        lines = [f"{self.command} {self.path}", ""] + [f"{k}: {v}" for k, v in self.headers.items()]
        body = ("\n".join(lines) + "\n").encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "3000"))
    print(f"echo app on http://localhost:{port}")
    HTTPServer(("127.0.0.1", port), Echo).serve_forever()
