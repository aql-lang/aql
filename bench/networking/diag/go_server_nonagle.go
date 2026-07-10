package main

import (
	"bufio"
	"net"
	"os"
)

func main() {
	ln, _ := net.Listen("tcp", "127.0.0.1:"+os.Args[1])
	conn, _ := ln.Accept()
	conn.(*net.TCPConn).SetNoDelay(false) // Nagle ON — the suspected AQL behaviour
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		w.Write(line)
		w.Flush()
	}
}
