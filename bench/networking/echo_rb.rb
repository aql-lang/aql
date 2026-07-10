# TCP echo round-trip benchmark in plain Ruby (socket + Thread).
# Same protocol as echo_aql.aql: one persistent connection, newline-framed,
# 20000 timed round-trips (+1 warm-up). TCP_NODELAY enabled on both ends to
# match Go/AQL defaults.
require "socket"

N = 20_000
HOST = "127.0.0.1"
PORT = 19_089
MSG = "hello world\n"

srv = TCPServer.new(HOST, PORT)
ready = Queue.new
Thread.new do
  ready << true
  conn = srv.accept
  conn.setsockopt(Socket::IPPROTO_TCP, Socket::TCP_NODELAY, 1)
  conn.sync = true
  (N + 1).times do
    line = conn.gets("\n")
    break if line.nil?
    conn.write(line) # echo verbatim
  end
  conn.close
end
ready.pop

sock = TCPSocket.new(HOST, PORT)
sock.setsockopt(Socket::IPPROTO_TCP, Socket::TCP_NODELAY, 1)
sock.sync = true

# Warm-up (not timed).
sock.write(MSG)
sock.gets("\n")

start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
N.times do
  sock.write(MSG)
  sock.gets("\n")
end
dur = Process.clock_gettime(Process::CLOCK_MONOTONIC) - start
printf("Ruby %d round-trips  %.1f ms  %.1f req/s\n", N, dur * 1000.0, N / dur)
sock.close
