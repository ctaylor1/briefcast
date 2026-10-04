"""Exercise real HTTP ordering without ML, credentials or Docker."""

import importlib.util
import json
import threading
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest

spec = importlib.util.spec_from_file_location(
    "whisperx_acceptance_proxy",
    Path(__file__).resolve().parents[1] / "scripts/whisperx_acceptance_proxy.py",
)
proxy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(proxy)


@pytest.mark.parametrize("boundary", ["result", "ack"])
def test_result_delivery_and_ack_forwarding_are_gated(tmp_path, boundary):
    forwarded = threading.Event()

    class Worker(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def reply(self):
            forwarded.set()
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"ok":true}')

        do_GET = reply
        do_POST = reply

    worker = ThreadingHTTPServer(("127.0.0.1", 0), Worker)
    server = ThreadingHTTPServer(("127.0.0.1", 0), proxy.Proxy)
    server.control = tmp_path
    server.upstream = worker.server_address
    (tmp_path / "mode").write_text(boundary)
    threads = [threading.Thread(target=s.serve_forever) for s in (worker, server)]
    for thread in threads:
        thread.start()
    try:
        with ThreadPoolExecutor(max_workers=1) as pool:
            path = "/v1/jobs/fixture/" + boundary
            request = urllib.request.Request(
                "http://127.0.0.1:" + str(server.server_port) + path,
                method="GET" if boundary == "result" else "POST",
            )
            response = pool.submit(urllib.request.urlopen, request, timeout=5)
            deadline = time.monotonic() + 3
            while not (tmp_path / "reached.json").exists() and time.monotonic() < deadline:
                time.sleep(0.01)
            assert json.loads((tmp_path / "reached.json").read_text()) == {
                "boundary": boundary,
                "path": path,
            }
            assert forwarded.is_set() == (boundary == "result")
            assert not response.done()
            (tmp_path / "mode").write_text("off")
            with response.result(timeout=3) as result:
                assert json.loads(result.read()) == {"ok": True}
            assert forwarded.is_set()
    finally:
        (tmp_path / "mode").write_text("off")
        for service, thread in zip((worker, server), threads, strict=True):
            service.shutdown()
            service.server_close()
            thread.join()


def test_unreleased_gate_fails_closed_without_request_data(tmp_path):
    (tmp_path / "mode").write_text("ack")
    assert not proxy.gate(tmp_path, "ack", "/v1/jobs/fixture/ack", timeout=0.01)
    assert json.loads((tmp_path / "reached.json").read_text()) == {
        "boundary": "ack",
        "path": "/v1/jobs/fixture/ack",
    }
