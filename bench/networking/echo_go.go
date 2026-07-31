// TCP echo round-trip benchmark in plain Go.
// Same protocol as echo_boru.boru: one persistent connection, newline-framed,
// 20000 timed round-trips (+1 warm-up).
package main

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

const n = 20000

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:19098")
	if err != nil {
		panic(err)
	}
	// Server: echo n+1 newline-framed lines.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			panic(err)
		}
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		for i := 0; i < n+1; i++ {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			w.Write(line)
			w.Flush()
		}
	}()

	conn, err := net.Dial("tcp", "127.0.0.1:19098")
	if err != nil {
		panic(err)
	}
	r := bufio.NewReader(conn)
	msg := []byte("hello world\n")

	// Warm-up (not timed).
	conn.Write(msg)
	r.ReadBytes('\n')

	start := time.Now()
	for i := 0; i < n; i++ {
		conn.Write(msg)
		r.ReadBytes('\n')
	}
	dur := time.Since(start)
	ms := float64(dur.Microseconds()) / 1000.0
	fmt.Printf("Go   %d round-trips  %.1f ms  %.1f req/s\n", n, ms, float64(n)/dur.Seconds())
}
