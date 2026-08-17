import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

recv()  # hello
send({"type": "ready"})
send({"type": "request", "id": "1", "capability": "env.get", "params": {"name": "WIRE_ECHO_VAR"}})
resp = recv()
val = resp.get("result", {}).get("value", "<denied:%s>" % resp.get("code"))
send({"type": "log", "level": "info", "message": "WIRE_ECHO_VAR=" + val})
send({"type": "done", "exitCode": 0})
