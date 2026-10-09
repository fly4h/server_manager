#!/usr/bin/env python3
"""Minimal fake Docker Engine REST API for local development.

The real daemon is not available on every machine (no Docker / no nested
virtualisation), but the Docker module in server-manager only needs a few
endpoints to be exercised. Serve those with stdlib only:

    python tools/fake-docker.py 2375

Then run the server with DOCKER_HOST=tcp://127.0.0.1:2375
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

CONTAINERS = [
    {
        "Id": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6",
        "Names": ["/openlist"],
        "Image": "openlist:latest",
        "State": "running",
        "Status": "Up 3 days",
    },
    {
        "Id": "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1",
        "Names": ["/nginx"],
        "Image": "nginx:alpine",
        "State": "running",
        "Status": "Up 5 hours",
    },
    {
        "Id": "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
        "Names": ["/redis"],
        "Image": "redis:7",
        "State": "exited",
        "Status": "Exited (0) 2 days ago",
    },
]

IMAGES = [
    {
        "Id": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
        "RepoTags": ["openlist:latest"],
        "Size": 104857600,
        "Created": 1700000000,
    },
    {
        "Id": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
        "RepoTags": ["nginx:alpine"],
        "Size": 41943040,
        "Created": 1700000100,
    },
]

ROUTES = {
    "/containers/json": CONTAINERS,
    "/images/json": IMAGES,
    "/volumes": {"Volumes": [{"Name": "openlist-data", "Driver": "local"}]},
    "/networks": {"Networks": [{"Name": "bridge", "Driver": "bridge"}]},
    "/_ping": "OK",
}


class Handler(BaseHTTPRequestHandler):
    def _send(self, status, payload):
        body = payload.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _handle(self):
        path = self.path.split("?", 1)[0]
        if path in ROUTES:
            data = ROUTES[path]
            payload = data if isinstance(data, str) else json.dumps(data)
            self._send(200, payload)
            return
        if path.endswith("/logs"):
            # 8-byte multiplexed frame per chunk: [stream, 0,0,0][size be32]
            body = b""
            for line in [b"fake container log line 1\n", b"fake container log line 2\n"]:
                body += b"\x01\x00\x00\x00" + len(line).to_bytes(4, "big") + line
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self._send(200, "{}")

    do_GET = _handle

    def do_POST(self):
        self._send(200, "{}")

    def do_DELETE(self):
        self._send(200, "{}")

    def log_message(self, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 2375
    print("fake-docker listening on 127.0.0.1:%d" % port, flush=True)
    HTTPServer(("127.0.0.1", port), Handler).serve_forever()
