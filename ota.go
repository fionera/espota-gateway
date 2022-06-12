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
	c <- "Got connection\n"
	defer close(c)

	b := bytes.NewReader(data)
	c <- "Sending chunks\n"
	for i := 0; true; i++ {
		c <- fmt.Sprintf("\r%d", i)
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := io.CopyN(conn, b, 1024); err != nil {
			if !errors.Is(err, io.EOF) {
				c <- "\nError sending data\n"
				return
			}
		}

		buf := make([]byte, 10)
		_, _ = conn.Read(buf)
		if bytes.Contains(buf, []byte("OK")) {
			c <- "\nDone\n"
			return
		}

		if b.Len() == 0 {
			break
		}
	}
	c <- "\n"

	buf := make([]byte, 32)
	_, _ = conn.Read(buf)
	if !bytes.Contains(buf, []byte("OK")) {
		c <- fmt.Sprintf("Error Uploading: %s (%x)\n", buf, buf)
		return
	}

	c <- "Flashing complete\n"
}
