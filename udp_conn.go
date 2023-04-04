package nethub

import (
	"net"
)

type udpConn struct {
	Addr     net.Addr
	listener *udpListener
}

func (n *udpConn) SendMessage(msg []byte) error {
	return n.listener.sendMessage(msg, n.Addr)
}

func (n *udpConn) SendMessageDirect(msg []byte) error {
	return n.listener.sendDirectMessage(msg, n.Addr)
}

func (n *udpConn) GetAuth() interface{} {
	return nil
}

func (n *udpConn) Close() {

}

func (n *udpConn) IsClosed() bool {
	return false
}

func (n *udpConn) ListenToOnDisconnect(f func(data interface{})) {

}

func (n *udpConn) ListenToOnMessage(f func(data interface{})) {

}
