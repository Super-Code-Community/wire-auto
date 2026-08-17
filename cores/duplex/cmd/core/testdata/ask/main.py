import sys, json
def send(o): sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())
recv()  # hello
send({"type": "ready"})
send({"type": "prompt", "id": "1", "message": "name?"})
resp = recv()
name = resp.get("result", {}).get("value", "?")
send({"type": "log", "level": "info", "message": "hello " + name})
send({"type": "done", "exitCode": 0, "result": {"name": name}})
