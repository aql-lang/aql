package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

const n = 20000

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:"+os.Args[1])
	if err != nil {
		panic(err)
	}
	r := bufio.NewReader(conn)
	msg := []byte("hello world\n")
	conn.Write(msg)
	r.ReadBytes('\n')
	start := time.Now()
	for i := 0; i < n; i++ {
		conn.Write(msg)
		r.ReadBytes('\n')
	}
	dur := time.Since(start)
	fmt.Printf("go-client  %d RT  %.1f ms  %.1f req/s\n", n, float64(dur.Microseconds())/1000.0, float64(n)/dur.Seconds())
	conn.Close()
}
