"""Private fixture proxy: stop result delivery or ACK forwarding before an app kill.

Only the isolated acceptance harness starts this service. It forwards to the
fixed Compose worker, never logs headers/bodies, and fails closed if a gate is
not released within 20 seconds. No production deployment uses this script.
"""

import argparse
import http.client
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


def gate(control, boundary, path, timeout=20):
    if (control / "mode").read_text() != boundary:
        return True
    marker = control / "reached.json"
    temporary = control / "reached.tmp"
    temporary.write_text(json.dumps({"boundary": boundary, "path": path}))
    temporary.replace(marker)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if (control / "mode").read_text() != boundary:
            return True
        time.sleep(0.05)
    return False


class Proxy(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass  # Authorization and private transcript paths must not be logged.

    def forward(self):
        if not self.path.startswith(("/v1/", "/health/")) or "\r" in self.path or "\n" in self.path:
            self.send_error(400)
            return
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if size < 0 or size > 1024 * 1024 or self.headers.get("Transfer-Encoding"):
                self.send_error(413)
                return
            body = self.rfile.read(size)
            if self.command == "POST" and self.path.endswith("/ack"):
                if not gate(self.server.control, "ack", self.path):
                    self.send_error(503)
                    return
            upstream = http.client.HTTPConnection(*self.server.upstream, timeout=10)
            try:
                headers = {"Content-Type": "application/json"}
                if self.headers.get("Authorization"):
                    headers["Authorization"] = self.headers["Authorization"]
                upstream.request(self.command, self.path, body, headers)
                response = upstream.getresponse()
                data = response.read(67108865)
                if len(data) > 67108864:
                    self.send_error(502)
                    return
                if (
                    self.command == "GET"
                    and self.path.endswith("/result")
                    and response.status == 200
                ):
                    if not gate(self.server.control, "result", self.path):
                        self.send_error(503)
                        return
                self.send_response(response.status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
            finally:
                upstream.close()
        except (OSError, ValueError, http.client.HTTPException):
            # A killed app closes its connection; do not emit request details.
            self.close_connection = True

    do_GET = forward
    do_POST = forward
    do_PUT = forward


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--control-dir", type=Path, required=True)
    args = parser.parse_args()
    server = ThreadingHTTPServer(("0.0.0.0", 8091), Proxy)
    server.control = args.control_dir
    server.upstream = ("whisperx", 8090)
    server.serve_forever()


if __name__ == "__main__":
    main()
