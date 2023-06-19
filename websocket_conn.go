package nethub

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func DialWebsocket(addr string) (*WebsocketConn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, err
	}
	return NewWebsocketConn(conn), nil
}

type WebsocketConn struct {
	ctx     context.Context
	cancel  context.CancelFunc
	id      string
	txQueue chan []byte
	sync.RWMutex
	isClosed  bool
	LoginData []byte

	PongWait     time.Duration
	PingInterval time.Duration
	WriteWait    time.Duration
	EnableLog    bool

	OnMessage    *EventTarget
	OnError      *EventTarget
	OnDisconnect *EventTarget
	Conn         *websocket.Conn
	properties   sync.Map

	writeMtx sync.Mutex
}

func (t *WebsocketConn) GetProperty(property string) (interface{}, bool) {
	return t.properties.Load(property)
}

func (t *WebsocketConn) SetProperty(property string, value interface{}) {
	t.properties.Store(property, value)
}

func (t *WebsocketConn) RemoteAddr() net.Addr {
	return t.Conn.RemoteAddr()
}

// websocket缓冲size大小
var WebsocketTxQueueLen = 100

func NewWebsocketConn(conn *websocket.Conn) *WebsocketConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebsocketConn{
		Conn:         conn,
		ctx:          ctx,
		txQueue:      make(chan []byte, WebsocketTxQueueLen),
		isClosed:     false,
		cancel:       cancel,
		id:           uuid.New().String(),
		OnMessage:    newEventTarget(),
		OnDisconnect: newEventTarget(),
		OnError:      newEventTarget(),
	}
}

var WsWritePkts uint64
var WsWriteByte uint64

func (t *WebsocketConn) StartReadWrite() {
	t.StartRead()
	t.StartWrite()
}

func (t *WebsocketConn) StartWrite() {
	go func() { //发送数据
		defer t.Close()
		for {
			select {
			case <-t.ctx.Done():
				return
			case data := <-t.txQueue:
				if err := t.SendMessageDirect(data); err != nil {
					log.Println(fmt.Sprintf(`websocket write error:%v`, err.Error()))
					t.OnError.RiseEvent(err)
					return
				}
			}
		}
	}()
}

func (t *WebsocketConn) StartRead() {
	go func() { //接收数据
		defer t.Close()
		for {
			select {
			case <-t.ctx.Done():
				return
			default:
				msg, err := t.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("websocket server read error: %v", err)
						t.OnError.RiseEvent(err)
					}
					if _, ok := err.(*websocket.CloseError); !ok {
						log.Printf("websocket server read error: %v", err)
						t.OnError.RiseEvent(err)
					}
					return
				}
				t.OnMessage.RiseEvent(msg)
			}
		}
	}()
}

func (t *WebsocketConn) ReadMessage() ([]byte, error) {
	_, data, err := t.Conn.ReadMessage()
	return data, err
}

func (t *WebsocketConn) SendMessageDirect(msg []byte) error {
	t.writeMtx.Lock()
	defer t.writeMtx.Unlock()
	atomic.AddUint64(&WsWriteByte, uint64(len(msg)))
	atomic.AddUint64(&WsWritePkts, 1)
	if t.WriteWait != 0 {
		t.Conn.SetWriteDeadline(time.Now().Add(t.WriteWait))
	}
	return t.Conn.WriteMessage(1, msg)
}

func (t *WebsocketConn) SendMessage(message []byte) error {
	select {
	case <-t.ctx.Done():
		return errors.New("ws connection closed when send buff msg")
	case t.txQueue <- message:
		return nil
	default: //队列满了丢弃消息
		if t.EnableLog {
			log.Println(fmt.Sprintf("ws[%v]发送队列满了丢弃消息[%v]", t.RemoteAddr().String(), string(message)))
		}
		return nil
	}
}

func (t *WebsocketConn) ListenToOnDisconnect(listener func(data interface{})) {
	t.OnDisconnect.AddEventListener(listener)
}

func (t *WebsocketConn) ListenToOnMessage(listener func(data interface{})) {
	t.OnMessage.AddEventListener(listener)
}

func (t *WebsocketConn) Close() {
	t.Lock()
	defer t.Unlock()
	if t.isClosed {
		return
	}
	t.isClosed = true
	t.cancel()
	t.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "close"), time.Now().Add(time.Second*3))
	t.Conn.Close()
	go t.OnDisconnect.RiseEvent(nil)
}

func (t *WebsocketConn) IsClosed() bool {
	t.RLock()
	defer t.RUnlock()
	return t.isClosed
}
