package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
)

type command int

const (
	Flash command = 0
	SpiFS command = 100
)

func (g *gateway) sendInvitation(c chan string, ip net.IP, cmd command, payload []byte) error {
	var conn net.Conn
	for i := 0; i < 10; i++ {
		cn, err := net.Dial("udp", ip.String()+":8266")
		if err != nil {
			c <- fmt.Sprintf("try %d/10: %s", i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		conn = cn
	}
	if conn == nil {
		return fmt.Errorf("failed to connect")
	}

	_ = conn.SetDeadline(time.Now().Add(time.Second * 10))

	port := g.firmwareListener.Addr().(*net.TCPAddr).Port
	if cmd == SpiFS {
		port = g.spifsListener.Addr().(*net.TCPAddr).Port
	}

	sum := md5.Sum(payload)
	msg := fmt.Sprintf("%d %d %d %s\n", cmd, port, len(payload), hex.EncodeToString(sum[:]))
	if _, err := conn.Write([]byte(msg)); err != nil {
		return err
	}

	buf := make([]byte, 37)
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	if !strings.HasPrefix(string(buf), "OK") {
		return fmt.Errorf("invalid invitation response: %s", buf)
	}

	return nil
}
