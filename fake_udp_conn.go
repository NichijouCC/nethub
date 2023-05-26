package nethub

import (
	"net"
	"sync/atomic"
)

type fakeUdpConn struct {
	Addr     net.Addr
	listener *udpListener
	beClosed atomic.Bool
}

func (n *fakeUdpConn) SendMessage(msg []byte) error {
	return n.listener.sendMessage(msg, n.Addr)
}

func (n *fakeUdpConn) SendMessageDirect(msg []byte) error {
	return n.listener.sendDirectMessage(msg, n.Addr)
}

func (n *fakeUdpConn) GetAuth() interface{} {
	return nil
}

func (n *fakeUdpConn) Close() {
	n.beClosed.Store(true)
}

func (n *fakeUdpConn) IsClosed() bool {
	return n.beClosed.Load()
}

func (n *fakeUdpConn) RemoteAddr() net.Addr {
	return n.Addr
}

func (n *fakeUdpConn) ListenToOnMessage(f func(data interface{})) {

}
