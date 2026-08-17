// zero-shim: speaks the wire protocol raw over stdio, no SDK import.
const chunks = [];
let buf = "";
const pending = [];

process.stdin.setEncoding("utf8");

process.stdin.on("data", (d) => {
  buf += d;
  let i;
  while ((i = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, i);
    buf = buf.slice(i + 1);
    if (line.length === 0) continue;
    const msg = JSON.parse(line);
    const waiter = pending.shift();
    if (waiter) waiter(msg);
    else chunks.push(msg);
  }
});

function recv() {
  if (chunks.length) return Promise.resolve(chunks.shift());
  return new Promise((res) => pending.push(res));
}
function send(o) {
  process.stdout.write(JSON.stringify(o) + "\n");
}

(async () => {
  await recv(); // hello
  send({ type: "ready" });
  send({ type: "request", id: "1", capability: "env.get", params: { name: "USER" } });
  const resp = await recv();
  const val = resp.result ? resp.result.value : "<denied:" + resp.code + ">";
  send({ type: "log", level: "info", message: "USER=" + val });
  send({ type: "done", exitCode: 0, result: { user: val } });
  process.exit(0);
})();
