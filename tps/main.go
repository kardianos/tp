package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/kardianos/service"
)

var log service.Logger

type Config struct {
	LocalPort int
}

var config *Config = &Config{
	LocalPort: 30541,
}

var (
	action = flag.String("action", "", "Control the service")
)

func main() {
	flag.Parse()

	sc := &service.Config{
		Name: "tps",
	}
	a := &app{}
	s, err := service.New(a, sc)
	if err != nil {
		fmt.Println("failed to create service: %v", err)
		os.Exit(1)
	}

	log, err = s.Logger(nil)
	if err != nil {
		fmt.Println("error opening logger: %v", err)
		os.Exit(1)
	}
	if len(*action) > 0 {
		err = service.Control(s, *action)
		if err != nil {
			fmt.Printf("available actions: %q\n", service.ControlAction)
			flag.PrintDefaults()
			os.Exit(2)
		}
		return
	}
	err = s.Run()
	if err != nil {
		log.Errorf("runtime error: %v", err)
	}
}

type app struct {
}

func (a *app) Start(s service.Service) error {
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", config.LocalPort))
	if err != nil {
		return err
	}
	go a.run(listen)
	return nil
}
func (a *app) run(listen net.Listener) {
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Warningf("failed to accept connection: %v", err)
			continue
		}
		go forward(conn)
	}
}
func (a *app) Stop(s service.Service) error {
	return nil
}

func forward(local net.Conn) {
	defer local.Close()
	var err error
	sizeBytes := make([]byte, 4)
	_, err = local.Read(sizeBytes)
	if err != nil {
		log.Warningf("failed to read size: %v", err)
		return
	}
	size := int(binary.LittleEndian.Uint32(sizeBytes))
	remoteAddr := make([]byte, size)
	_, err = local.Read(remoteAddr)
	if err != nil {
		log.Warningf("failed to read remote address: %v", err)
		return
	}

	remote, err := net.Dial("tcp", string(remoteAddr))
	if err != nil {
		log.Warningf("failed to dial %q: %v", remoteAddr, err)
		return
	}
	defer remote.Close()

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
