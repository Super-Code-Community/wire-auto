"""Тонкий шим wire-auto для python: говорит протокол моста (JSON Lines)."""
import json
import sys


class Script:
    def __init__(self, stdin=None, stdout=None):
        self._in = stdin if stdin is not None else sys.stdin
        self._out = stdout if stdout is not None else sys.stdout
        self.hello = None

    def start(self):
        line = self._in.readline()
        self.hello = json.loads(line)
        self._send({"type": "ready"})
        return self.hello

    def log(self, message, level="info"):
        self._send({"type": "log", "level": level, "message": message})

    def done(self, exit_code=0, result=None):
        msg = {"type": "done", "exitCode": exit_code}
        if result is not None:
            msg["result"] = result
        self._send(msg)

    def _send(self, obj):
        # compact separators keep the wire format byte-consistent with the Go side
        self._out.write(json.dumps(obj, separators=(",", ":")) + "\n")
        self._out.flush()
