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

var publicIP = mustEnv("PUBLIC_IP")

func (g *gateway) sendInvitation(c chan string, ip net.IP, cmd command, payload []byte) error {
	port := g.firmwareListener.Addr().(*net.TCPAddr).Port
	if cmd == SpiFS {
		port = g.spifsListener.Addr().(*net.TCPAddr).Port
	}

	for i := 0; i < 10; i++ {
		d := net.Dialer{
			LocalAddr: &net.UDPAddr{IP: net.ParseIP(publicIP)},
		}

		conn, err := d.Dial("udp", ip.String()+":8266")
		if err != nil {
			return err
		}

		_ = conn.SetDeadline(time.Now().Add(time.Second * 10))

		sum := md5.Sum(payload)
		msg := fmt.Sprintf("%d %d %d %s\n", cmd, port, len(payload), hex.EncodeToString(sum[:]))
		if _, err := conn.Write([]byte(msg)); err != nil {
			c <- fmt.Sprintf("try %d/10: %s\n", i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}

		buf := make([]byte, 37)
		if _, err := conn.Read(buf); err != nil {
			c <- fmt.Sprintf("try %d/10: %s\n", i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if !strings.HasPrefix(string(buf), "OK") {
			return fmt.Errorf("invalid invitation response: %s", buf)
		}

		return nil
	}

	return fmt.Errorf("failed inviting device")
}
