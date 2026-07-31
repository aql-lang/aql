// TCP echo round-trip benchmark in plain TypeScript (Node net).
// Same protocol as echo_boru.boru: one persistent connection, newline-framed,
// 20000 timed round-trips (+1 warm-up). TCP_NODELAY enabled on both ends
// to match Go/boru defaults.
import * as net from "node:net";

const N = 20000;
const PORT = 19097;
const MSG = Buffer.from("hello world\n");

function readLine(sock: net.Socket): Promise<Buffer> {
  return new Promise((resolve) => {
    const onData = (chunk: Buffer) => {
      // Messages are small and echoed one-at-a-time, so a chunk ending in
      // '\n' is a complete line for this benchmark.
      if (chunk[chunk.length - 1] === 0x0a) {
        sock.removeListener("data", onData);
        resolve(chunk);
      }
    };
    sock.on("data", onData);
  });
}

async function main() {
  const server = net.createServer((conn) => {
    conn.setNoDelay(true);
    conn.on("data", (chunk) => {
      conn.write(chunk); // echo verbatim
    });
  });
  await new Promise<void>((r) => server.listen(PORT, "127.0.0.1", r));

  const sock = net.connect({ port: PORT, host: "127.0.0.1" });
  sock.setNoDelay(true);
  await new Promise<void>((r) => sock.once("connect", r));

  // Warm-up (not timed).
  sock.write(MSG);
  await readLine(sock);

  const start = process.hrtime.bigint();
  for (let i = 0; i < N; i++) {
    sock.write(MSG);
    await readLine(sock);
  }
  const durNs = process.hrtime.bigint() - start;
  const ms = Number(durNs) / 1e6;
  const reqs = (N * 1e9) / Number(durNs);
  console.log(`TS   ${N} round-trips  ${ms.toFixed(1)} ms  ${reqs.toFixed(1)} req/s`);

  sock.destroy();
  server.close();
}

main();
