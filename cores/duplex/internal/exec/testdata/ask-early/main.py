import sys, json
def send(o): sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())
recv()  # hello
send({"type": "prompt", "id": "1", "message": "too early"})  # before ready → violation
