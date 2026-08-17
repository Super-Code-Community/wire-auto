import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

recv()  # hello — never imports any SDK; speaks the protocol raw
send({"type": "ready"})
send({"type": "request", "id": "1", "capability": "env.get", "params": {"name": "USER"}})
resp = recv()
val = resp.get("result", {}).get("value", "<denied:%s>" % resp.get("code"))
send({"type": "log", "level": "info", "message": "USER=" + val})
send({"type": "done", "exitCode": 0, "result": {"user": val}})
