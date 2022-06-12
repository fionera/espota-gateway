package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

func (g *gateway) handleConn(conn net.Conn) {
	defer conn.Close()

	ip := conn.RemoteAddr().(*net.TCPAddr).IP
	log.Println("Conn from", ip)

	g.mtx.Lock()
	data, ok := g.payloadMap[ip.String()]
	if !ok {
		log.Println("no data for", ip)
		return
	}

	c := g.chanMap[ip.String()]
	g.mtx.Unlock()
	c <- "Got connection"
	defer close(c)

	b := bytes.NewReader(data)
	for {
		c <- "sending chunk"
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		_, err := io.CopyN(conn, b, 1024)
		if err != nil {
			if errors.Is(err, io.EOF) {
				goto read
			}
			c <- "error writing"
			return
		}

	read:
		buf := make([]byte, 10)
		_, _ = conn.Read(buf)
		if !bytes.Contains(buf, []byte("OK")) {
			c <- "Error Uploading:" + fmt.Sprintf("%s (%x)", buf, buf)
			return
		}

		if b.Len() == 0 {
			break
		}
	}

	c <- "Done"
}
