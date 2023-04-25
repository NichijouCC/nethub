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
	ctx      context.Context
	cancel   context.CancelFunc
	id       string
	sendChan chan []byte
	sync.RWMutex
	isClosed bool
	auth     interface{}

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

func (t *WebsocketConn) GetAuth() interface{} {
	return t.auth
}

// websocket缓冲size大小
var WebsocketSendChanSize = 100

func NewWebsocketConn(conn *websocket.Conn) *WebsocketConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebsocketConn{
		Conn:         conn,
		ctx:          ctx,
		sendChan:     make(chan []byte, WebsocketSendChanSize),
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
	go func() { //接收数据
		defer t.Close()
		for {
			select {
			case <-t.ctx.Done():
				return
			default:
				msg, err := t.readOnePacket()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("websocket server read error: %v", err)
						t.OnError.RiseEvent(err)
					}
					if _, ok := err.(*websocket.CloseError); !ok {
						log.Printf("websocket server read error: %v", err)
						t.OnError.RiseEvent(err)
					}
					t.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, err.Error()), time.Now().Add(time.Second*3))
					return
				}
				t.OnMessage.RiseEvent(msg)
			}
		}
	}()

	go func() { //发送数据
		defer t.Close()
		for {
			select {
			case <-t.ctx.Done():
				return
			case data := <-t.sendChan:
				if err := t.writeOnePacket(data); err != nil {
					log.Println(fmt.Sprintf(`websocket write error:%v`, err.Error()))
					t.OnError.RiseEvent(err)
					t.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, err.Error()), time.Now().Add(time.Second*3))
					return
				}
			}
		}
	}()

	if t.PingInterval > 0 {
		t.Conn.SetReadDeadline(time.Now().Add(t.PongWait))
		t.Conn.SetPongHandler(func(appData string) error {
			if t.EnableLog {
				log.Println("收到pong消息", t.RemoteAddr().String(), time.Now().String())
			}
			t.Conn.SetReadDeadline(time.Now().Add(t.PongWait))
			return nil
		})
		go func() {
			ticker := time.NewTicker(t.PingInterval)
			for {
				select {
				case <-t.ctx.Done():
					return
				case <-ticker.C:
					t.writeMtx.Lock()
					if t.WriteWait != 0 {
						t.Conn.SetWriteDeadline(time.Now().Add(t.WriteWait))
					}
					if t.EnableLog {
						log.Println("发送ping消息", t.RemoteAddr().String(), time.Now().String())
					}
					if err := t.Conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
						log.Println(fmt.Sprintf(`websocket write ping error:%v`, err.Error()))
						t.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, err.Error()), time.Now().Add(time.Second*3))
						t.writeMtx.Unlock()
						return
					}
					t.writeMtx.Unlock()
				}
			}
		}()
	}
}

func (t *WebsocketConn) writeOnePacket(msg []byte) error {
	t.writeMtx.Lock()
	defer t.writeMtx.Unlock()
	atomic.AddUint64(&WsWriteByte, uint64(len(msg)))
	atomic.AddUint64(&WsWritePkts, 1)
	if t.WriteWait != 0 {
		t.Conn.SetWriteDeadline(time.Now().Add(t.WriteWait))
	}
	return t.Conn.WriteMessage(1, msg)
}

func (t *WebsocketConn) readOnePacket() ([]byte, error) {
	_, data, err := t.Conn.ReadMessage()
	return data, err
}

func (t *WebsocketConn) SendMessage(message []byte) error {
	select {
	case <-t.ctx.Done():
		return errors.New("ws connection closed when send buff msg")
	case t.sendChan <- message:
		return nil
	default: //队列满了丢弃消息
		if t.EnableLog {
			log.Println(fmt.Sprintf("ws[%v]发送队列满了丢弃消息[%v]", t.RemoteAddr().String(), string(message)))
		}
		return nil
	}
}

func (t *WebsocketConn) SendMessageDirect(msg []byte) error {
	return t.writeOnePacket(msg)
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
	t.OnDisconnect.RiseEvent(nil)
	t.Conn.Close()
}

func (t *WebsocketConn) IsClosed() bool {
	t.RLock()
	defer t.RUnlock()
	return t.isClosed
}
