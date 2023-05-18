package nethub

import (
	"crypto/tls"
	"errors"
	reuse "github.com/libp2p/go-reuseport"
	"log"
	"net"
	"sync"
	"time"
)

type TcpServer struct {
	Addr string
	sync.RWMutex
	clients map[*TcpConn]struct{}

	Opts               *ServerOptions
	OnReceiveMessage   func(message []byte, conn *TcpConn)
	OnError            func(err error)
	OnClientConnect    func(conn *TcpConn)
	OnClientDisconnect func(conn *TcpConn)
}

type authOptions struct {
	Timeout   time.Duration
	CheckFunc func(loginInfo []byte, conn IConn) (interface{}, error)
}

type ServerOptions struct {
	Tls  *tls.Config
	Auth *authOptions
}

func WithTls(tls *tls.Config) func(opt *ServerOptions) {
	return func(opt *ServerOptions) {
		opt.Tls = tls
	}
}

func WithAuth(auth *authOptions) func(opt *ServerOptions) {
	return func(opt *ServerOptions) {
		opt.Auth = auth
	}
}

func NewTcpServer(address string, args ...func(opt *ServerOptions)) *TcpServer {
	opts := &ServerOptions{}
	for _, el := range args {
		el(opts)
	}
	tcpServer := &TcpServer{
		Addr:    address,
		Opts:    opts,
		clients: map[*TcpConn]struct{}{},
	}
	return tcpServer
}

func (s *TcpServer) ListenAndServe(listenerCount int) error {
	log.Println("tcp Server try listen to ", s.Addr)
	for i := 0; i < listenerCount; i++ {
		listener, err := reuse.Listen("tcp", s.Addr)
		if err != nil {
			return err
		}
		if s.Opts.Tls != nil {
			listener = tls.NewListener(listener, s.Opts.Tls)
		}
		go func() {
			for {
				conn, e := listener.Accept()
				if e != nil {
					if ne, ok := e.(net.Error); ok && ne.Temporary() {
						log.Printf("accept temp err: %v", ne)
						continue
					}
					log.Printf("accept err: %v", e)
					if s.OnError != nil {
						s.OnError(e)
					}
					return
				}

				go func() {
					tcpConn := NewTcpConn(conn)
					//检查认证数据包
					if s.Opts.Auth != nil {
						beOk, err := s.checkConnAuth(tcpConn, s.Opts.Auth)
						if err != nil {
							tcpConn.Close()
							return
						}
						if !beOk {
							tcpConn.Close()
							return
						}
					}
					tcpConn.OnMessage.AddEventListener(func(data interface{}) {
						if s.OnReceiveMessage != nil {
							s.OnReceiveMessage(data.([]byte), tcpConn)
						}
					})
					tcpConn.OnError.AddEventListener(func(data interface{}) {
						if s.OnError != nil {
							s.OnError(data.(error))
						}
					})
					tcpConn.OnDisconnect.AddEventListener(func(data interface{}) {
						s.handleClientDisconnect(tcpConn)
					})
					s.handleClientConnect(tcpConn)
					tcpConn.StartReadWrite()
				}()
			}
		}()
	}
	return nil
}

func (s *TcpServer) checkConnAuth(conn *TcpConn, config *authOptions) (bool, error) {
	onEnd := make(chan struct{})
	ok, err, authInfo := false, error(nil), []byte(nil)
	go func() {
		authInfo, err = conn.ReadOnePacket()
		if err != nil {
			close(onEnd)
		} else {
			conn.auth, err = config.CheckFunc(authInfo, conn)
			if err != nil {
				close(onEnd)
			} else {
				ok = true
				close(onEnd)
			}
		}
	}()
	select {
	case <-time.After(config.Timeout):
		return false, errors.New("认证超时")
	case <-onEnd:
		return ok, err
	}
}

func (s *TcpServer) handleClientConnect(conn *TcpConn) {
	s.Lock()
	s.clients[conn] = struct{}{}
	s.Unlock()
	log.Printf("NetTcp connected from: %v", conn.RemoteAddr().String())
	if s.OnClientConnect != nil {
		s.OnClientConnect(conn)
	}
}

func (s *TcpServer) handleClientDisconnect(conn *TcpConn) {
	s.Lock()
	delete(s.clients, conn)
	s.Unlock()
	log.Println("NetTcp disconnected from: " + conn.RemoteAddr().String())
	if s.OnClientDisconnect != nil {
		s.OnClientDisconnect(conn)
	}
}
