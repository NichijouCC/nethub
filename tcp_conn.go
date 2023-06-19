package nethub

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var TcpWritePkts uint64
var TcpWriteByte uint64
var TcpReadPkts uint64
var TcpReadByte uint64

type TcpConn struct {
	ctx       context.Context
	cancel    context.CancelFunc
	id        string
	txQueue   chan []byte
	closeMu   sync.RWMutex
	isClosed  bool
	LoginData []byte

	OnMessage    *EventTarget
	OnError      *EventTarget
	OnDisconnect *EventTarget
	Conn         net.Conn
	properties   sync.Map
}

func (t *TcpConn) GetProperty(property string) (interface{}, bool) {
	return t.properties.Load(property)
}

func (t *TcpConn) SetProperty(property string, value interface{}) {
	t.properties.Store(property, value)
}

func (t *TcpConn) RemoteAddr() net.Addr {
	return t.Conn.RemoteAddr()
}

// tcp连接发送队列大小
var TcpConnTxQueueLen = 100

// tcp连接管理
func NewTcpConn(conn net.Conn) *TcpConn {
	ctx, cancel := context.WithCancel(context.Background())
	conn.(*net.TCPConn).SetKeepAlive(true)
	conn.(*net.TCPConn).SetKeepAlivePeriod(20 * time.Second)
	return &TcpConn{
		Conn:         conn,
		ctx:          ctx,
		txQueue:      make(chan []byte, TcpConnTxQueueLen),
		isClosed:     false,
		cancel:       cancel,
		id:           uuid.New().String(),
		OnMessage:    newEventTarget(),
		OnError:      newEventTarget(),
		OnDisconnect: newEventTarget(),
	}
}

func (t *TcpConn) StartReadWrite() {
	t.StartRead()
	t.StartWrite()
}

func (t *TcpConn) StartRead() {
	go func() { //接收数据
		defer t.Close()
		for {
			select {
			case <-t.ctx.Done():
				return
			default:
				msg, err := packetRead(t.Conn)
				if err != nil {
					if err == io.EOF {
						log.Println(fmt.Sprintf(`TCP收到结束信号`))
					} else {
						t.OnError.RiseEvent(err)
						log.Println(fmt.Sprintf(`TCPread error:%v`, err.Error()))
					}
					return
				}
				//log.Println("tcp read:", string(msg))
				t.OnMessage.RiseEvent(msg)
			}
		}
	}()
}

func (t *TcpConn) StartWrite() {
	go func() { //发送数据
		for {
			select {
			case <-t.ctx.Done():
				return
			case data := <-t.txQueue:
				//log.Println("tcp send:", string(data))
				if err := packetWrite(t.Conn, data); err != nil {
					log.Println(fmt.Sprintf(`TCP write error:%v`, err.Error()))
					t.OnError.RiseEvent(err)
					return
				}
			}
		}
	}()
}

func (t *TcpConn) ReadMessage() ([]byte, error) {
	return packetRead(t.Conn)
}

func (t *TcpConn) SendMessage(message []byte) error {
	select {
	case <-t.ctx.Done():
		return nil
	case t.txQueue <- message:
		return nil
	default:
		return fmt.Errorf("tcp发送队列已满，丢弃消息,to:%v", t.RemoteAddr().String())
	}
}

func (t *TcpConn) SendMessageDirect(msg []byte) error {
	return packetWrite(t.Conn, msg)
}

func (t *TcpConn) Type() string {
	return "tcp"
}

func (t *TcpConn) Close() {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()
	if t.isClosed {
		return
	}
	t.Conn.Close()
	t.isClosed = true
	t.cancel()
	go t.OnDisconnect.RiseEvent(nil)
}

func (t *TcpConn) IsClosed() bool {
	t.closeMu.RLock()
	defer t.closeMu.RUnlock()
	return t.isClosed
}

func (t *TcpConn) ListenToOnDisconnect(listener func(interface{})) {
	t.OnDisconnect.AddEventListener(listener)
}

func (t *TcpConn) ListenToOnMessage(listener func(interface{})) {
	t.OnMessage.AddEventListener(listener)
}

// 连接tcp服务器
func DialTcp(addr string) (*TcpConn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return NewTcpConn(conn), nil
}

func packetRead(reader io.Reader) ([]byte, error) {
	bufMsgLen := make([]byte, 4)
	// read len
	if _, err := io.ReadFull(reader, bufMsgLen); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(bufMsgLen)
	// data
	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(reader, msgData); err != nil {
		return nil, err
	}
	atomic.AddUint64(&TcpReadByte, uint64(msgLen+4))
	atomic.AddUint64(&TcpReadPkts, uint64(1))
	return msgData, nil
}

func packetWrite(writer io.Writer, msg []byte) error {
	var lengthBuf = make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(msg)))
	packet := append(lengthBuf, msg...)
	n, err := writer.Write(packet)
	atomic.AddUint64(&TcpWriteByte, uint64(n))
	atomic.AddUint64(&TcpWritePkts, uint64(1))
	return err
}
