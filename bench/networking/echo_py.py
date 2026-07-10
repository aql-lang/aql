#!/usr/bin/env python3
# TCP echo round-trip benchmark in plain Python (socket + threading).
# Same protocol as echo_aql.aql: one persistent connection, newline-framed,
# 20000 timed round-trips (+1 warm-up). TCP_NODELAY enabled on both ends to
# match Go/AQL defaults.
import socket
import threading
import time

N = 20000
HOST = "127.0.0.1"
PORT = 19090
MSG = b"hello world\n"


def server(ready: threading.Event) -> None:
    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((HOST, PORT))
    srv.listen(1)
    ready.set()
    conn, _ = srv.accept()
    conn.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    rf = conn.makefile("rb")  # buffered reader for newline framing
    for _ in range(N + 1):
        line = rf.readline()
        if not line:
            break
        conn.sendall(line)  # echo verbatim
    conn.close()
    srv.close()


def main() -> None:
    ready = threading.Event()
    threading.Thread(target=server, args=(ready,), daemon=True).start()
    ready.wait()

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.connect((HOST, PORT))
    sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    rf = sock.makefile("rb")

    # Warm-up (not timed).
    sock.sendall(MSG)
    rf.readline()

    start = time.perf_counter()
    for _ in range(N):
        sock.sendall(MSG)
        rf.readline()
    dur = time.perf_counter() - start
    print(f"Py   {N} round-trips  {dur * 1000.0:.1f} ms  {N / dur:.1f} req/s")
    sock.close()


if __name__ == "__main__":
    main()
