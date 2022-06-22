package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type Payload struct {
	IP       net.IP
	Firmware []byte
	SpiFS    []byte
}

type gateway struct {
	spifsListener    net.Listener
	firmwareListener net.Listener
	mtx              sync.Mutex
	chanMap          map[string]chan string
	payloadMap       map[string][]byte
}

func newGateway(fwAddr, spifAddr string) (*gateway, error) {
	g := gateway{
		chanMap:    make(map[string]chan string),
		payloadMap: make(map[string][]byte),
	}

	firmwareListener, err := newListener(fwAddr, g.handleConn)
	if err != nil {
		return nil, err
	}
	g.firmwareListener = firmwareListener

	spiffsListener, err := newListener(spifAddr, g.handleConn)
	if err != nil {
		return nil, err
	}
	g.spifsListener = spiffsListener

	return &g, nil
}

func newListener(addr string, handler func(conn net.Conn)) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Fatal(err)
			}

			go handler(conn)
		}
	}()

	return l, nil
}

func mustEnv(k string) string {
	v, ok := os.LookupEnv(k)
	if !ok {
		panic(fmt.Sprintf("Missing ENV: %s", k))
	}
	return v
}

func main() {
	g, err := newGateway(":8181", ":8182")
	if err != nil {
		log.Panic(err)
	}

	log.Println("startup")
	http.Handle("/", g)
	http.ListenAndServe(":8180", nil)
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("invalid method"))
		return
	}

	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("payload too large"))
		return
	}

	var p Payload
	if v, ok := r.MultipartForm.Value["ip"]; ok {
		if len(v) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid ip field"))
			return
		}

		p.IP = net.ParseIP(v[0])
		if p.IP == nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("failed to parse ip"))
			return
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing ip"))
		return
	}

	for key, fileHeaders := range r.MultipartForm.File {
		if len(fileHeaders) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("duplicate file"))
			return
		}

		file, err := fileHeaders[0].Open()
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("cant open file"))
			return
		}
		all, err := io.ReadAll(file)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("failed reading file"))
			return
		}

		switch key {
		case "firmware":
			p.Firmware = all
		case "spiffs":
			p.SpiFS = all
		default:
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("invalid file name"))
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	c := g.handleFlash(&p)
	for s := range c {
		w.Write([]byte(s))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (g *gateway) handleFlash(p *Payload) <-chan string {
	c := make(chan string)

	go func() {
		defer close(c)
		defer func() {
			g.mtx.Lock()
			delete(g.chanMap, p.IP.String())
			delete(g.payloadMap, p.IP.String())
			g.mtx.Unlock()
		}()

		if p.Firmware != nil {
			stageChan := make(chan string)
			go func() {
				<-time.NewTimer(10 * time.Minute).C
				close(stageChan)
			}()

			g.mtx.Lock()
			g.chanMap[p.IP.String()] = stageChan
			g.payloadMap[p.IP.String()] = p.Firmware
			g.mtx.Unlock()

			c <- "Sending Flash Invite\n"
			err := g.sendInvitation(c, p.IP, Flash, p.Firmware)
			if err != nil {
				c <- err.Error()
				return
			}

			// We start a second channel for the stage and copy the results
			for s := range stageChan {
				c <- s
			}
		}

		if p.SpiFS != nil {
			stageChan := make(chan string)
			go func() {
				<-time.NewTimer(10 * time.Minute).C
				close(stageChan)
			}()

			g.mtx.Lock()
			g.chanMap[p.IP.String()] = stageChan
			g.payloadMap[p.IP.String()] = p.SpiFS
			g.mtx.Unlock()

			c <- "Sending SpiFS Invite\n"
			err := g.sendInvitation(c, p.IP, SpiFS, p.SpiFS)
			if err != nil {
				c <- err.Error()
				return
			}

			// We start a second channel for the stage and copy the results
			for s := range stageChan {
				c <- s
			}
		}

		c <- "Thanks for using the OTA-Gateway\n"
	}()

	return c
}
