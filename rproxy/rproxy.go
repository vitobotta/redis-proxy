package rproxy

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

////////////////////////////////////////
// RedisProxyConfig

type RedisProxyConfig struct {
	UplinkAddr      string `json:"uplink_addr"`
	ListenOn        string `json:"listen_on"`
	AdminOn         string `json:"admin_on"`
	ReadTimeLimitMs int64  `json:"read_time_limit_ms"`
	LogMessages     bool   `json:"log_messages"`
}

func LoadConfig(fname string) (*RedisProxyConfig, error) {
	configJson, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	var config RedisProxyConfig
	return &config, json.Unmarshal(configJson, &config)
}

////////////////////////////////////////
// RedisProxy

type RedisProxy struct {
	config_file string
	config      *RedisProxyConfig
	controller  *ProxyController
}

func NewRedisProxy(config_file string) (*RedisProxy, error) {
	config, err := LoadConfig(config_file)
	if err != nil {
		return nil, err
	}
	proxy := &RedisProxy{
		config_file: config_file,
		config:      config,
		controller:  NewProxyController()}
	return proxy, nil
}

func (proxy *RedisProxy) Run() error {
	listener, err := net.Listen("tcp", proxy.config.ListenOn)
	if err != nil {
		return err
	}

	log.Println("Listening on", proxy.config.ListenOn)

	go proxy.watchSignals()
	go proxy.controller.run(proxy) // TODO: clean this up when getting rid of circular dep
	go proxy.publishAdminInterface()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go proxy.handleClient(NewRespConn(conn, 0, proxy.config.LogMessages))
	}
}

func (proxy *RedisProxy) ReloadConfig() {
	newConfig, err := LoadConfig(proxy.config_file)
	if err != nil {
		log.Printf("Got an error while loading %s: %s.  Keeping old config.", proxy.config_file, err)
		return
	}

	if err := proxy.verifyNewConfig(newConfig); err != nil {
		log.Printf("Can not reload into new config: %s.  Keeping old config.", err)
		return
	}
	proxy.config = newConfig
}

func (proxy *RedisProxy) watchSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	for {
		s := <-c
		log.Printf("Got signal: %v, reloading config\n", s)
		proxy.controller.Reload()
	}
}

func (proxy *RedisProxy) verifyNewConfig(newConfig *RedisProxyConfig) error {
	config := proxy.config
	if config.ListenOn != newConfig.ListenOn {
		return errors.New("New config must have the same listen_on address as the old one.")
	}
	if config.AdminOn != newConfig.AdminOn {
		return errors.New("New config must have the same admin_on address as the old one.")
	}
	return nil
}

func (proxy *RedisProxy) handleClient(cliConn *RespConn) {
	log.Printf("Handling new client: connection from %s", cliConn.RemoteAddr())

	uplinkAddr := ""
	var uplinkConn *RespConn

	defer func() {
		cliConn.Close()
		if uplinkConn != nil {
			uplinkConn.Close()
		}
	}()

	for {
		req, err := cliConn.ReadObject()
		if err != nil {
			log.Printf("Read error: %v\n", err)
			return
		}

		resp, err := proxy.controller.ExecuteCall(func() ([]byte, error) {
			currUplinkAddr := proxy.config.UplinkAddr
			if uplinkAddr != currUplinkAddr {
				uplinkAddr = currUplinkAddr
				if uplinkConn != nil {
					uplinkConn.Close()
				}
				uplinkConn, err = RespDial("tcp", uplinkAddr,
					proxy.config.ReadTimeLimitMs,
					proxy.config.LogMessages,
				)
				if err != nil {
					return nil, err
				}
			}

			uplinkConn.Write(req)
			return uplinkConn.ReadObject()
		})
		if err != nil {
			log.Printf("Error: %v\n", err)
			return
		}

		cliConn.Write(resp)
	}
}
