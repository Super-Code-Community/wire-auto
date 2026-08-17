import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()

sys.stdin.readline()  # hello
send({"type": "ready"})
send({"type": "log", "level": "info", "message": "no request here"})
send({"type": "done", "exitCode": 0})
