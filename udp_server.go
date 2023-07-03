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
	OnReceiveMessage func(data []byte, addr net.Addr, lis *udpListener)
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
		newUdpListener(c, func(data []byte, addr net.Addr, lis *udpListener) {
			if u.OnReceiveMessage != nil {
				u.OnReceiveMessage(data, addr, lis)
			}
		})
	}
	return nil
}

type udpListener struct {
	txQueue chan *writeMessage
	socket  net.PacketConn
}

func newUdpListener(socket net.PacketConn, onMessage func(data []byte, addr net.Addr, lis *udpListener)) *udpListener {
	lis := &udpListener{
		txQueue: make(chan *writeMessage, UdpTxQueueLen),
		socket:  socket,
	}
	go func() {
		defer socket.Close()
		buf := make([]byte, packetBufSize)
		for {
			if len(buf) < UDPPacketSize {
				buf = make([]byte, packetBufSize, packetBufSize)
			}
			nBytes, addr, err := socket.ReadFrom(buf)
			if err != nil {
				log.Printf("udp read error %s", err)
				continue
			}
			msg := buf[:nBytes]
			buf = buf[nBytes:]
			atomic.AddUint64(&UdpReadPkts, 1)
			atomic.AddUint64(&UdpReadByte, uint64(nBytes))
			//log.Println("udp read", len(msg), msg)
			onMessage(msg, addr, lis)
		}
	}()
	go func() {
		for m := range lis.txQueue {
			atomic.AddInt64(&UdpWriteQueue, -1)
			n, err := socket.WriteTo(m.msg, m.addr)
			if err != nil {
				log.Printf("udp write error %s", err)
			}
			atomic.AddUint64(&UdpWritePkts, 1)
			atomic.AddUint64(&UdpWriteByte, uint64(n))
		}
	}()
	return lis
}

func (u *udpListener) sendMessage(data []byte, addr net.Addr) error {
	atomic.AddInt64(&UdpWriteQueue, 1)
	select {
	case u.txQueue <- &writeMessage{msg: data, addr: addr}:
		return nil
	default:
		atomic.AddInt64(&UdpWriteQueue, -1)
		return fmt.Errorf("udp发送队列已满，丢弃消息,to:%v", addr.String())
	}
}

func (u *udpListener) sendDirectMessage(data []byte, addr net.Addr) error {
	_, err := u.socket.WriteTo(data, addr)
	return err
}
