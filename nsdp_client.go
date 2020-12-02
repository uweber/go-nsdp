package main

import (
	"gs308e/nsdp"
	"log"
	"math/rand"
	"net"
	"time"
)

// get first non-loopback address, intf-name and mac
func getSelfIntfAndIp() (string, string, []byte, error) {
	intfs, err := net.Interfaces()
	if err != nil {
		return "", "", nil, err
	}

	for _, intf := range intfs {
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String(), intf.Name, intf.HardwareAddr, nil
				}
			}
		}
	}

	return "", "", nil, nil
}

type Client struct {
	anyAddr    *net.UDPAddr
	targetAddr *net.UDPAddr
	intfName   string
	intfHwAddr []byte
	conn       *net.UDPConn
	seq        uint16
}

func NewClient() (*Client, error) {
	selfAddrStr, intfName, intfHwAddr, err := getSelfIntfAndIp()
	if err != nil {
		return nil, err
	}

	selfAddr, err := net.ResolveUDPAddr("udp", selfAddrStr+":63321")
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", selfAddr)
	if err != nil {
		return nil, err
	}

	anyAddr, err := net.ResolveUDPAddr("udp", "255.255.255.255:63322")
	if err != nil {
		return nil, err
	}

	targetAddr, err := net.ResolveUDPAddr("udp", "192.168.132.170:63322")
	if err != nil {
		return nil, err
	}

	// to avoid ignore msg, set random sequence number
	rand.Seed(time.Now().UnixNano())
	seq := uint16(rand.Intn(0xffff))
	log.Println(seq)

	return &Client{
		anyAddr:    anyAddr,
		targetAddr: targetAddr,
		conn:       conn,
		intfHwAddr: intfHwAddr,
		intfName:   intfName,
		seq:        seq,
	}, nil
}

func (c *Client) SendRecvMsg(msg nsdp.Msg) *nsdp.Msg {
	c.seq = c.seq + 1

	recvCh := make(chan bool, 1)
	buf := make([]byte, 65535)
	readLen := 0
	go func() {
		readLen, _, _ = c.conn.ReadFrom(buf)
		log.Println("recv", readLen, buf[:readLen])
		recvCh <- true
	}()

	retry := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for retry < 3 {
		select {
		case <-recvCh:
			return nsdp.ParseMsg(buf[:readLen])
		case <-ticker.C:
			b := msg.Bytes()
			log.Println("send", b)
			writeLen, err := c.conn.WriteTo(b, c.anyAddr)
			if err != nil {
				log.Println(writeLen, err)
			}
			retry += 1
		}
	}

	return nil
}

func (c *Client) Read(msg ...nsdp.TLV) *nsdp.Msg {
	m := nsdp.Msg(nsdp.DefaultMsg)
	m.Op = 1
	m.Seq = c.seq
	m.HostMac = c.intfHwAddr
	m.Body = nsdp.Body{Body: msg}

	return c.SendRecvMsg(m)
}

func (c *Client) Write(msg ...nsdp.TLV) *nsdp.Msg {
	m := nsdp.Msg(nsdp.DefaultMsg)
	m.Op = 3
	m.Seq = c.seq
	m.HostMac = c.intfHwAddr
	m.DeviceMac = []byte{0x44, 0xa5, 0x6e, 0x11, 0x11, 0x11}
	m.Body = nsdp.Body{Body: msg}

	return c.SendRecvMsg(m)
}
