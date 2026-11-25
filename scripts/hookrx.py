import http.server, json, sys
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0))).decode()
        print(json.dumps({"event": self.headers.get("X-Vaultly-Event"), "ts": self.headers.get("X-Vaultly-Timestamp"),
                          "sig": self.headers.get("X-Vaultly-Signature"), "body": body}), flush=True)
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def log_message(self, *a): pass
http.server.HTTPServer(("0.0.0.0", 9000), H).serve_forever()
