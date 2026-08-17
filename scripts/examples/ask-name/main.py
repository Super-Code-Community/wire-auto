import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

_id = 0
def prompt(message):
    global _id
    _id += 1
    send({"type": "prompt", "id": str(_id), "message": message})
    resp = recv()
    return resp.get("result", {}).get("value", "")

recv()  # hello — говорим на протоколе напрямую, без SDK
send({"type": "ready"})

name = prompt("Как тебя зовут?")
send({"type": "log", "level": "info", "message": "Привет, %s!" % (name or "аноним")})
send({"type": "done", "exitCode": 0, "result": {"name": name}})
