package nethub

import (
	"fmt"
	reuse "github.com/libp2p/go-reuseport"
	"log"
	"net"
	"sync/atomic"
)

const (
	UDPPacketSize = 1024 * 4
	packetBufSize = 1024 * 1024 // 1 MB
)

type writeMessage struct {
	addr net.Addr
	msg  []byte
}

var UdpReadPkts uint64
var UdpReadByte uint64
var UdpWritePkts uint64
var UdpWriteByte uint64
var UdpWriteQueue int64

type UdpServer struct {
	OnReceiveMessage func(data []byte, conn *udpConn)
	address          string
}

func NewUdpServer(address string) *UdpServer {
	return &UdpServer{
		address: address,
	}
}

func (u *UdpServer) ListenAndServe(listenerCount int) error {
	log.Println("udp server try listen to ", u.address)
	for i := 0; i < listenerCount; i++ {
		c, err := reuse.ListenPacket("udp", u.address)
		//c, err := net.ListenPacket("udp", u.address)
		if err != nil {
			panic(err)
		}
		newUdpListener(c, func(data []byte, conn *udpConn) {
			if u.OnReceiveMessage != nil {
				u.OnReceiveMessage(data, conn)
			}
		})
	}
	return nil
}

type udpListener struct {
	sendCh chan *writeMessage
	conn   net.PacketConn
}

func newUdpListener(conn net.PacketConn, onMessage func(data []byte, conn *udpConn)) *udpListener {
	listener := &udpListener{
		sendCh: make(chan *writeMessage, UdpSendChanSize),
		conn:   conn,
	}
	go func() {
		defer conn.Close()
		buf := make([]byte, packetBufSize)
		for {
			if len(buf) < UDPPacketSize {
				buf = make([]byte, packetBufSize, packetBufSize)
			}
			nBytes, addr, err := conn.ReadFrom(buf)
			if err != nil {
				log.Printf("udp read error %s", err)
				continue
			}
			msg := buf[:nBytes]
			buf = buf[nBytes:]
			atomic.AddUint64(&UdpReadPkts, 1)
			atomic.AddUint64(&UdpReadByte, uint64(nBytes))
			onMessage(msg, &udpConn{addr, listener})
		}
	}()
	go func() {
		for m := range listener.sendCh {
			atomic.AddInt64(&UdpWriteQueue, -1)
			n, err := conn.WriteTo(m.msg, m.addr)
			if err != nil {
				log.Printf("udp write error %s", err)
			}
			atomic.AddUint64(&UdpWritePkts, 1)
			atomic.AddUint64(&UdpWriteByte, uint64(n))
		}
	}()
	return listener
}

func (u *udpListener) sendMessage(data []byte, addr net.Addr) error {
	atomic.AddInt64(&UdpWriteQueue, 1)
	select {
	case u.sendCh <- &writeMessage{msg: data, addr: addr}:
		return nil
	default:
		atomic.AddInt64(&UdpWriteQueue, -1)
		return fmt.Errorf("udp发送队列已满，丢弃消息,to:%v", addr.String())
	}
}

func (u *udpListener) sendDirectMessage(data []byte, addr net.Addr) error {
	_, err := u.conn.WriteTo(data, addr)
	return err
}
