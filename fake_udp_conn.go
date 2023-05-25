package nethub

import (
	"net"
)

type fakeUdpConn struct {
	Addr     net.Addr
	listener *udpListener
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

}

func (n *fakeUdpConn) ListenToOnMessage(f func(data interface{})) {

}
