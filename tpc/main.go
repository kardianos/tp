package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

var (
	remote = flag.String("remote", "", "remote address to dial")
	local  = flag.Int("local", 5000, "local port to listen to")
	tcs    = flag.String("tcs", "localhost:30541", "address of the tcs server")
)

var header []byte

func main() {
	flag.Parse()

	if len(*remote) == 0 || len(*tcs) == 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	header = make([]byte, 4+len(*remote))
	binary.LittleEndian.PutUint32(header, uint32(len(*remote)))
	copy(header[4:], *remote)

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", *local))
	if err != nil {
		log.Fatalf("failed to listen to port %d: %v", *local, err)
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}
		go forward(conn)
	}
}
func forward(local net.Conn) {
	defer local.Close()
	var err error

	remote, err := net.Dial("tcp", *tcs)
	if err != nil {
		log.Printf("failed to dial tcs %q: %v", *tcs, err)
		return
	}
	defer remote.Close()

	_, err = remote.Write(header)
	if err != nil {
		log.Printf("failed to write header: %v", err)
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		io.Copy(local, remote)
		wg.Done()
		local.Close()
	}()

	go func() {
		io.Copy(remote, local)
		wg.Done()
		remote.Close()
	}()
	wg.Wait()
}
