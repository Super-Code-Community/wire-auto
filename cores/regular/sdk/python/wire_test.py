import io
import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import wire  # noqa: E402


class ShimTest(unittest.TestCase):
    def run_script(self, hello):
        stdin = io.StringIO(json.dumps(hello) + "\n")
        stdout = io.StringIO()
        s = wire.Script(stdin=stdin, stdout=stdout)
        got_hello = s.start()
        s.log("hello from python")
        s.done(0)
        lines = [json.loads(l) for l in stdout.getvalue().splitlines() if l]
        return got_hello, lines

    def test_handshake_and_messages(self):
        hello, lines = self.run_script({"type": "hello", "protocol": 1, "coreApi": 1})
        self.assertEqual(hello["coreApi"], 1)
        self.assertEqual(lines[0], {"type": "ready"})
        self.assertEqual(lines[1], {"type": "log", "level": "info", "message": "hello from python"})
        self.assertEqual(lines[2], {"type": "done", "exitCode": 0})


if __name__ == "__main__":
    unittest.main()
