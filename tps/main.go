package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/kardianos/osext"
	"github.com/kardianos/service"
	"github.com/kardianos/tp/internal"
)

const configFileName = "tps.config"

var log service.Logger

type Config struct {
	LocalPort int

	KeyFile string
	key     []byte
}

var config *Config = &Config{
	LocalPort: 30541,
}

var (
	action = flag.String("action", "", "Control the service")
)

/*
	RDP Magic: 03 00
	RDP host name length at byte: 130
	RDP host name ASCII follows length byte.

*/

func main() {
	flag.Parse()

	hasFile, err := loadConfig()
	if hasFile == true && err != nil {
		fmt.Println("failed to read config file %q: %v", configFileName, err)
		os.Exit(1)
	}

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
			fmt.Printf("Control action %q failed: %v\n", *action, err)
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
	var err error
	var listen net.Listener

	listenOn := fmt.Sprintf(":%d", config.LocalPort)

	if len(config.KeyFile) != 0 {
		log.Info("Secure connection")
		var key []byte
		key, err = ioutil.ReadFile(config.KeyFile)
		if err != nil {
			return err
		}
		listen, err = internal.Listen(key, "tcp", listenOn, time.Second*30)
	} else {
		listen, err = net.Listen("tcp", listenOn)
	}
	if err != nil {
		return err
	}
	log.Infof("Listening on %q", listenOn)
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

	log.Info("Accept %v", local.RemoteAddr())

	var err error
	sizeBytes := make([]byte, 4)
	local.SetReadDeadline(time.Now().Add(time.Second))
	_, err = local.Read(sizeBytes)
	if err != nil {
		log.Warningf("failed to read size: %v", err)
		return
	}
	size := int(binary.LittleEndian.Uint32(sizeBytes))
	remoteAddrBuf := make([]byte, size)
	_, err = local.Read(remoteAddrBuf)
	if err != nil {
		log.Warningf("failed to read remote address: %v", err)
		return
	}

	local.SetReadDeadline(time.Time{})

	remoteAddr := string(remoteAddrBuf)

	if remoteAddr == "$$PING$$" {
		log.Info("Got PING")
		local.Write([]byte("$$PONG$$"))
		log.Info("Sent PONG")
		return
	}

	dialer := &net.Dialer{
		KeepAlive: time.Second * 30,
		Timeout:   time.Second * 10,
	}
	remote, err := dialer.Dial("tcp", remoteAddr)
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

func loadConfig() (hasFile bool, err error) {
	folder, err := osext.ExecutableFolder()
	if err != nil {
		return false, err
	}
	configBytes, err := ioutil.ReadFile(filepath.Join(folder, configFileName))
	if err != nil {
		return false, nil
	}
	err = toml.Unmarshal(configBytes, &config)
	return true, err
}
