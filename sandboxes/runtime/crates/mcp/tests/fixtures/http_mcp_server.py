#!/usr/bin/env python3
"""Minimal stateless Streamable HTTP MCP fixture with auth assertions.

The fixture deliberately implements the wire protocol directly so the client
integration test is not merely exercising rmcp against another rmcp process.
It accepts paths for no auth, static header, static bearer, user OAuth, and
OAuth client credentials.
"""

import json
import queue
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit


EXPECTED_AUTH = {
    "/noauth": (None, None),
    "/names": (None, None),
    "/slow": (None, None),
    "/static": ("x-api-key", "static-test-key"),
    "/static-bearer": ("authorization", "Bearer static-bearer-token"),
    "/oauth": ("authorization", "Bearer oauth-user-token"),
    "/client-credentials": ("authorization", "Bearer machine-access-token"),
}
LEGACY_SESSIONS = {}
LEGACY_SESSIONS_LOCK = threading.Lock()
LEGACY_AUTH = "Bearer legacy-oauth-token"
INTEROP_TOOL_NAMES = [
    "records.list",
    "records/list",
    "records_list",
    "read_" + ("x" * 100),
]


class McpHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, _format, *_args):
        return

    def do_GET(self):
        if self.path == "/legacy-redirect":
            self.send_response(307)
            self.send_header("Location", "http://169.254.169.254/latest/meta-data/")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.path != "/legacy-sse":
            self._json(404, {"error": "unknown fixture path"})
            return
        if self.headers.get("authorization") != LEGACY_AUTH:
            self._json(401, {"error": "invalid authorization"})
            return
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        session_id = uuid.uuid4().hex
        messages = queue.Queue()
        with LEGACY_SESSIONS_LOCK:
            LEGACY_SESSIONS[session_id] = messages
        self.wfile.write(
            f"event: endpoint\ndata: /legacy-messages?sessionId={session_id}\n\n".encode(
                "utf-8"
            )
        )
        self.wfile.flush()
        try:
            while True:
                try:
                    response = messages.get(timeout=1)
                    payload = json.dumps(response).encode("utf-8")
                    self.wfile.write(b"event: message\ndata: " + payload + b"\n\n")
                except queue.Empty:
                    self.wfile.write(b": keep-alive\n\n")
                self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            return
        finally:
            with LEGACY_SESSIONS_LOCK:
                LEGACY_SESSIONS.pop(session_id, None)

    def do_POST(self):
        if self.path == "/streamable-redirect":
            self.send_response(307)
            self.send_header("Location", "http://169.254.169.254/latest/meta-data/")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.path.startswith("/legacy-messages?"):
            if self.headers.get("authorization") != LEGACY_AUTH:
                self._json(401, {"error": "invalid authorization"})
                return
            request = self._read_request()
            response = self._rpc_response(request, "/legacy-sse", "2024-11-05")
            if response is not None:
                session_id = parse_qs(urlsplit(self.path).query).get("sessionId", [""])[0]
                with LEGACY_SESSIONS_LOCK:
                    messages = LEGACY_SESSIONS.get(session_id)
                if messages is None:
                    self._json(410, {"error": "legacy session expired"})
                    return
                messages.put(response)
            self.send_response(202)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return

        expected = EXPECTED_AUTH.get(self.path)
        if expected is None:
            self._json(404, {"error": "unknown fixture path"})
            return
        header, value = expected
        if header is None:
            if self.headers.get("authorization") or self.headers.get("x-api-key"):
                self._json(401, {"error": "no-auth endpoint received credentials"})
                return
        elif self.headers.get(header) != value:
            self._json(401, {"error": f"invalid {header}"})
            return

        request = self._read_request()
        method = request.get("method")

        if method == "notifications/initialized":
            self.send_response(202)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        response = self._rpc_response(request, self.path, "2025-11-25")
        if response is None:
            self._json(400, {"error": "notification has no response"})
            return
        self._json(200, response, content_type="application/json")

    def _read_request(self):
        length = int(self.headers.get("content-length", "0"))
        return json.loads(self.rfile.read(length) or b"{}")

    def _rpc_response(self, request, endpoint, protocol_version):
        method = request.get("method")
        request_id = request.get("id")
        if request_id is None:
            return None
        if method == "initialize":
            requested_version = (request.get("params") or {}).get("protocolVersion")
            if requested_version != protocol_version:
                return self._rpc_error_value(
                    request_id,
                    -32602,
                    f"expected protocolVersion {protocol_version}, got {requested_version}",
                )
            result = {
                "protocolVersion": protocol_version,
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "hivy-auth-fixture", "version": "1.0.0"},
            }
        elif method == "tools/list":
            if endpoint == "/slow":
                time.sleep(1)
            if endpoint == "/names":
                result = {
                    "tools": [
                        {
                            "name": name,
                            "description": f"Interoperability fixture tool {name}",
                            "inputSchema": {"type": "object", "properties": {}},
                        }
                        for name in INTEROP_TOOL_NAMES
                    ]
                }
                return {"jsonrpc": "2.0", "id": request_id, "result": result}
            result = {
                "tools": [
                    {
                        "name": "echo",
                        "title": "Echo payload",
                        "description": "Echo a payload and report the authenticated fixture endpoint.",
                        "inputSchema": {
                            "type": "object",
                            "properties": {"message": {"type": "string"}},
                            "required": ["message"],
                        },
                        "outputSchema": {
                            "type": "object",
                            "properties": {
                                "message": {"type": "string"},
                                "endpoint": {"type": "string"},
                            },
                            "required": ["message", "endpoint"],
                        },
                    },
                    {
                        "name": "lookup_customer",
                        "description": "Find a customer by email address.",
                        "inputSchema": {
                            "type": "object",
                            "properties": {"email": {"type": "string"}},
                            "required": ["email"],
                        },
                    },
                ]
            }
        elif method == "tools/call":
            params = request.get("params") or {}
            if endpoint == "/names":
                raw_name = params.get("name")
                if raw_name not in INTEROP_TOOL_NAMES:
                    return self._rpc_error_value(request_id, -32602, "unknown tool")
                structured = {"raw_name": raw_name}
                result = {
                    "content": [{"type": "text", "text": json.dumps(structured)}],
                    "structuredContent": structured,
                    "isError": False,
                }
                return {"jsonrpc": "2.0", "id": request_id, "result": result}
            if params.get("name") != "echo":
                return self._rpc_error_value(request_id, -32602, "unknown tool")
            arguments = params.get("arguments") or {}
            structured = {
                "message": arguments.get("message"),
                "endpoint": endpoint,
                "authorization": self.headers.get("authorization"),
                "api_key": self.headers.get("x-api-key"),
            }
            result = {
                "content": [{"type": "text", "text": json.dumps(structured)}],
                "structuredContent": structured,
                "isError": False,
            }
        else:
            return self._rpc_error_value(request_id, -32601, f"unknown method {method}")
        return {"jsonrpc": "2.0", "id": request_id, "result": result}

    def _rpc_error_value(self, request_id, code, message):
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": code, "message": message},
        }

    def _json(self, status, value, content_type="application/json"):
        payload = json.dumps(value).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


server = ThreadingHTTPServer(("127.0.0.1", 0), McpHandler)
print(f"PORT={server.server_address[1]}", flush=True)
try:
    server.serve_forever()
except KeyboardInterrupt:
    pass
finally:
    server.server_close()
