package nethub

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"log"
	"net"
	"sync"
	"time"
)

type UdpClient struct {
	ctx          context.Context
	cancel       context.CancelFunc
	Conn         *net.UDPConn
	sessionId    string
	txQueue      chan []byte
	OnMessage    *EventTarget
	OnError      *EventTarget
	OnDisconnect *EventTarget
	sync.RWMutex
	isClosed   bool
	readBuffer []byte
}

// UDP发送队列大小
var UdpTxQueueLen = 100

func newUdpClient() *UdpClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &UdpClient{
		ctx:          ctx,
		cancel:       cancel,
		txQueue:      make(chan []byte, UdpTxQueueLen),
		OnMessage:    newEventTarget(),
		OnError:      newEventTarget(),
		OnDisconnect: newEventTarget(),
		readBuffer:   make([]byte, packetBufSize),
	}
}

func (n *UdpClient) StartReadWrite(heartbeatTimeout float64) {
	go func() { //接收数据
		defer n.Close()
		buf := make([]byte, packetBufSize)
		for {
			select {
			case <-n.ctx.Done():
				return
			default:
				if len(buf) < UDPPacketSize {
					buf = make([]byte, packetBufSize, packetBufSize)
				}
				nBytes, _, err := n.Conn.ReadFrom(buf) // 接收数据
				if err != nil {
					fmt.Println("read udp failed, err: ", err)
					continue
				}
				msg := buf[:nBytes]
				buf = buf[nBytes:]
				n.OnMessage.RiseEvent(msg)
			}
		}
	}()

	go func() { //发送数据
		defer n.Close()
		for {
			select {
			case <-n.ctx.Done():
				return
			case data := <-n.txQueue:
				if _, err := n.Conn.Write(data); err != nil {
					log.Println(fmt.Sprintf(`udp write error:%v`, err.Error()))
					n.OnError.RiseEvent(err)
					return
				}
			}
		}
	}()
}

func (n *UdpClient) SendMessage(msg []byte) error {
	log.Println(string(msg))
	n.txQueue <- []byte(fmt.Sprintf("%v@%v", string(msg), n.sessionId))
	return nil
}

func (n *UdpClient) SendMessageDirect(msg []byte) error {
	_, err := n.Conn.Write([]byte(fmt.Sprintf("%v@%v", string(msg), n.sessionId)))
	return err
}

func (n *UdpClient) ReadMessage() ([]byte, error) {
	if len(n.readBuffer) < UDPPacketSize {
		n.readBuffer = make([]byte, packetBufSize, packetBufSize)
	}
	nBytes, _, err := n.Conn.ReadFrom(n.readBuffer) // 接收数据
	if err != nil {
		fmt.Println("read udp failed, err: ", err)
		n.cancel()
		return nil, err
	}
	msg := n.readBuffer[:nBytes]
	n.readBuffer = n.readBuffer[nBytes:]
	return msg, err
}

func (n *UdpClient) Close() {
	n.Lock()
	defer n.Unlock()
	if n.isClosed {
		return
	}
	n.isClosed = true
	n.cancel()
	n.OnDisconnect.RiseEvent(nil)
	n.Conn.Close()
}

func (n *UdpClient) IsClosed() bool {
	n.RLock()
	defer n.RUnlock()
	return n.isClosed
}

func (n *UdpClient) GetAuth() interface{} {
	return nil
}

func (n *UdpClient) RemoteAddr() net.Addr {
	return n.Conn.RemoteAddr()
}

func (n *UdpClient) Type() string {
	return "udp"
}

func (n *UdpClient) ListenToOnDisconnect(f func(data interface{})) {
	n.OnDisconnect.AddEventListener(f)
}

func (n *UdpClient) ListenToOnMessage(f func(data interface{})) {
	n.OnMessage.AddEventListener(f)
}

func DialUdp(addr string) (*UdpClient, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
	logger.Info("udp connect", zap.String("rt", conn.LocalAddr().String()))
	client := newUdpClient()
	client.Conn = conn
	return client, nil
}

func DialHubUdp(addr string, params LoginParams, opts *ClientOptions) *Client {
	var client = newClient(nil, opts)
	client.beClient.Store(true)
	var tryConn func()
	tryConn = func() {
		logger.Info("Try进行udp连接..", zap.Any("addr", addr))
		conn, err := DialUdp(addr)
		if err != nil {
			logger.Info("udp连接失败", zap.Error(err))
			time.Sleep(time.Second * 3)
			go tryConn()
			return
		}
		client.conn = conn
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			logger.Info("udp断开连接")
			client.ClearAllSubTopics()
			time.Sleep(3 * time.Second)
			go tryConn()
		})
		conn.OnMessage.AddEventListener(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.StartReadWrite(5)
		for {
			err = client.Login(&params)
			if err != nil {
				logger.Error("登录失败", zap.Error(err), zap.Any("login params", params))
				time.Sleep(3 * time.Second)
			} else {
				break
			}
		}
	}
	go tryConn()
	return client
}
