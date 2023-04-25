package nethub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var Enqueue int64 = 0
var EnConsume int64 = 0

type ClientOptions struct {
	HeartbeatTimeout float64
	RetryTimeout     float64
	RetryInterval    float64
}

type Client struct {
	ctx                context.Context
	cancel             context.CancelCauseFunc
	ClientId           string
	sessionId          string
	Bucket             *Bucket
	conn               atomic.Value
	OnLogin            *EventTarget
	OnDisconnect       *EventTarget
	OnHeartbeatTimeout *EventTarget
	OnPingHandler      func(pkt *PingPacket)
	OnPongHandler      func(pkt *PongPacket)
	isClosed           atomic.Bool
	lastMessageTime    atomic.Value

	enqueueCh     chan *Packet
	packetHandler map[PacketTypeCode]func(pkt *Packet)

	*handlerMgr
	//正在处理的request,struct{}
	handlingReq sync.Map
	//等待Response回复,*flyPacket
	pendingResp sync.Map
	//等待Ack回复,*flyPacket
	pendingAck sync.Map

	//正在处理的stream,*Stream
	handlingStream sync.Map

	properties sync.Map
	*PubSub
	OnBroadcast func(pkt *BroadcastPacket)
	options     *ClientOptions
}

// 客户端消息(read后)处理队列缓冲大小
var clientEnqueueChanSize = 100

func newClient(clientId string, conn IConn, opts *ClientOptions) *Client {
	client := &Client{
		ClientId:        clientId,
		conn:            atomic.Value{},
		OnLogin:         newEventTarget(),
		OnDisconnect:    newEventTarget(),
		lastMessageTime: atomic.Value{},
		packetHandler:   map[PacketTypeCode]func(pkt *Packet){},
		enqueueCh:       make(chan *Packet, clientEnqueueChanSize),
		handlerMgr:      &handlerMgr{},
		PubSub:          newPubSub(ClientPubChanSize),
		options:         opts,
	}
	client.ctx, client.cancel = context.WithCancelCause(context.Background())
	if conn != nil {
		client.conn.Store(conn)
	}
	client.lastMessageTime.Store(time.Now())
	//包处理
	client.packetHandler[REQUEST_PACKET] = client.onReceiveRequest
	client.packetHandler[ACK_PACKET] = client.onReceiveAck
	client.packetHandler[RESPONSE_PACKET] = client.onReceiveResponse
	client.packetHandler[PUBLISH_PACKET] = client.onReceivePublish
	client.packetHandler[SUBSCRIBE_PACKET] = client.onReceiveSubscribe
	client.packetHandler[UNSUBSCRIBE_PACKET] = client.onReceiveSubscribe
	client.packetHandler[BROADCAST_PACKET] = client.onReceiveBroadcast
	client.packetHandler[STREAM_REQUEST_PACKET] = client.onReceiveStreamRequest
	client.packetHandler[STREAM_RESPONSE_PACKET] = client.onReceiveStreamResponse
	client.packetHandler[STREAM_CLOSE_PACKET] = client.onReceiveCloseStream
	client.packetHandler[PING_PACKET] = func(pkt *Packet) {
		if client.OnPingHandler != nil {
			client.OnPingHandler(pkt.PacketContent.(*PingPacket))
		}
		var content = PongPacket(*pkt.PacketContent.(*PingPacket))
		client.SendPacket(&content)
	}
	client.packetHandler[PONG_PACKET] = func(pkt *Packet) {
		if client.OnPongHandler != nil {
			client.OnPongHandler(pkt.PacketContent.(*PongPacket))
		}
	}

	go func() {
		for pkt := range client.enqueueCh {
			atomic.AddInt64(&Enqueue, -1)
			atomic.AddInt64(&EnConsume, 1)
			handler, ok := client.packetHandler[pkt.PacketType]
			if !ok {
				logger.Error("rpc通信包找不到handler", zap.Any("packet", string(pkt.RawData)))
				return
			}
			handler(pkt)
		}
	}()

	return client
}

func (m *Client) LoopCheckAlive() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		if m.isClosed.Load() {
			continue
		}
		if time.Now().Sub(m.lastMessageTime.Load().(time.Time)) > time.Millisecond*time.Duration(m.options.HeartbeatTimeout*1000*0.3) {
			//logger.Warn(fmt.Sprintf("客户端【%v】无数据,发送PING.", m.ClientId))
			var ping PingPacket = "PING"
			m.SendPacket(&ping)
		}
		if time.Now().Sub(m.lastMessageTime.Load().(time.Time)) > time.Millisecond*time.Duration(m.options.HeartbeatTimeout*1000) {
			logger.Error(fmt.Sprintf("客户端【%v】超时无数据,断开连接.", m.ClientId))
			m.Close()
		}
	}
}

func (m *Client) GetProperty(property string) (interface{}, bool) {
	return m.properties.Load(property)
}

func (m *Client) SetProperty(property string, value interface{}) {
	m.properties.Store(property, value)
}

func (m *Client) receivePacket(pkt *Packet) {
	pkt.Client = m
	atomic.AddInt64(&Enqueue, 1)
	m.lastMessageTime.Store(time.Now())
	m.enqueueCh <- pkt
}

func (m *Client) onReceiveRequest(pkt *Packet) {
	request := pkt.PacketContent.(*RequestRawPacket)
	if request.Id != "" {
		//收到请求就回复一个ack
		m.SendPacket(&AckPacket{Id: request.Id})
		//如果还未处理则创建处理
		if _, loaded := m.handlingReq.LoadOrStore(request.Id, struct{}{}); !loaded {
			go func() {
				defer m.handlingReq.Delete(request.Id)
				resp := &ResponsePacket{Id: request.Id}
				//执行request
				handler, ok := m.findRequestHandler(request.Method)
				if !ok {
					resp.Error = "无法找到对应方法"
				} else {
					result, err := handler.execute(pkt)
					if err != nil {
						logger.Error("request执行出错", zap.Error(err))
						resp.Error = err.Error()
					} else {
						resp.Result = result
					}
				}
				err := m.SendPacketWithRetry(resp)
				if err != nil {
					logger.Error("发送response出错", zap.Error(err))
				}
			}()
		}
	} else {
		if handler, ok := m.findRequestHandler(request.Method); ok {
			handler(pkt)
		}
	}
}

func (m *Client) onReceiveResponse(pkt *Packet) {
	resp := pkt.PacketContent.(*ResponsePacket)
	if resp.Id != "" {
		m.SendPacket(&AckPacket{Id: resp.Id})
	}
	if value, loaded := m.pendingResp.LoadAndDelete(resp.Id); loaded {
		f := value.(*flyPacket)
		if resp.Error != "" {
			f.err = errors.New(resp.Error)
		} else {
			f.result = resp.Result
		}
		close(f.resultBack)
	}
}

func (m *Client) onReceiveAck(pkt *Packet) {
	ack := pkt.PacketContent.(*AckPacket)
	if value, loaded := m.pendingAck.LoadAndDelete(ack.Id); loaded {
		f := value.(*flyPacket)
		close(f.ackBack)
	}
}

func (m *Client) onReceivePublish(pkt *Packet) {
	request := pkt.PacketContent.(*PublishRawPacket)
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	if m.Bucket != nil {
		request.Topic = fmt.Sprintf("%v/%v", m.ClientId, request.Topic)
		request.ClientId = m.ClientId
		m.Bucket.PubTopic(request, m)
	} else {
		m.PubTopic(request, m)
	}
}

func (m *Client) onReceiveSubscribe(sub *Packet) {
	request := sub.PacketContent.(*SubscribePacket)
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	m.SubTopic(NewTopicListener(request.Topic, func(pkt *PublishRawPacket, from *Client) {
		if request.Attributes != nil {
			var msg map[string]interface{}
			err := json.Unmarshal(pkt.Params, &msg)
			if err != nil {
				return
			}
			filter := make(map[string]interface{})
			for _, attribute := range request.Attributes {
				filter[attribute] = msg[attribute]
			}
			params, _ := json.Marshal(filter)
			pkt = &PublishRawPacket{Id: pkt.Id, Params: params, Topic: pkt.Topic, ClientId: pkt.ClientId}
		}
		if pkt.Id == "" {
			m.SendPacket(pkt)
		} else {
			go m.SendPacketWithRetry(pkt)
		}
	}))
}

func (m *Client) onReceiveUnsubscribe(pkt *Packet) {
	request := pkt.PacketContent.(*UnSubscribePacket)
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	m.UnsubTopic(request.Topic)
}

func (m *Client) onReceiveBroadcast(pkt *Packet) {
	request := pkt.PacketContent.(*BroadcastPacket)
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	if m.Bucket != nil {
		m.Bucket.Broadcast(pkt.RawData)
	} else if m.OnBroadcast != nil {
		m.OnBroadcast(request)
	}
}

func (m *Client) onReceiveStreamRequest(pkt *Packet) {
	request := pkt.PacketContent.(*StreamRequestRawPacket)
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	if value, loaded := m.handlingStream.Load(request.StreamId); loaded {
		value.(*Stream).OnRev <- request
		return
	}
	handler, ok := m.findStreamHandler(request.Method)
	if !ok {
		closePkt := &StreamClosePacket{StreamId: request.Id, Error: "无法找到对应方法"}
		go m.SendPacketDirect(closePkt)
		return
	}
	value, loaded := m.handlingStream.LoadOrStore(request.StreamId, newStream(request.StreamId, m))
	//如果还未建立stream则创建stream处理
	value.(*Stream).OnRev <- request
	if !loaded {
		go func() {
			stream := value.(*Stream)
			err := handler.execute(pkt, stream)
			if _, ok := m.handlingStream.LoadAndDelete(request.StreamId); !ok { //已被处理close
				return
			}
			closePkt := &StreamClosePacket{StreamId: request.StreamId}
			if err != nil {
				logger.Error("stream执行出错", zap.Error(err))
				closePkt.Error = err.Error()
			}
			err = m.SendPacketDirect(closePkt)
			if err != nil {
				logger.Error("发送StreamClosePacket出错", zap.Error(err))
			}
		}()
	}
}

func (m *Client) onReceiveStreamResponse(pkt *Packet) {
	req := pkt.PacketContent.(*StreamResponsePacket)
	if req.Id != "" {
		m.SendPacket(&AckPacket{Id: req.Id})
	}
	if handlingStream, loaded := m.handlingStream.Load(req.StreamId); loaded {
		handlingStream.(*Stream).OnRev <- req
	}
}

func (m *Client) onReceiveCloseStream(pkt *Packet) {
	req := pkt.PacketContent.(*StreamClosePacket)
	if req.Id != "" {
		m.SendPacket(&AckPacket{Id: req.Id})
	}
	if handlingStream, loaded := m.handlingStream.Load(req.StreamId); loaded {
		handlingStream.(*Stream).OnRev <- req
	}
}

// 发送Request,等待回复或超时
func (m *Client) Request(method string, params interface{}) (interface{}, error) {
	pkt := &RequestPacket{Id: uuid.New().String(), Method: method, Params: params}
	if value, loaded := m.pendingResp.Load(pkt.Id); loaded {
		request := value.(*flyPacket)
		<-request.resultBack
		return request.result, request.err
	}
	if value, loaded := m.pendingResp.LoadOrStore(pkt.Id, newFlyRequest()); !loaded {
		request := value.(*flyPacket)
		m.SendPacket(pkt)
		timeout := time.After(time.Second * time.Duration(m.options.RetryTimeout))
		for {
			select {
			case <-request.resultBack:
				return request.result, request.err
			case <-timeout:
				request.err = errors.New("超时未接收到RequestAck或者Response回复")
				close(request.resultBack)
				return nil, request.err
			}
		}
	} else {
		request := value.(*flyPacket)
		<-request.resultBack
		return request.result, request.err
	}
}

// 发送Request,重复发直到收到RequestAck或收到Request执行结果或超时(Request_timeout)
func (m *Client) RequestWithRetry(method string, params interface{}) (interface{}, error) {
	return m.RequestWithRetryByPacket(&RequestPacket{Id: uuid.New().String(), Method: method, Params: params})
}

// 发送Request,重复发直到收到RequestAck或收到Request执行结果或超时(Request_timeout)
func (m *Client) RequestWithRetryByPacket(pkt *RequestPacket) (interface{}, error) {
	if value, loaded := m.pendingResp.Load(pkt.Id); loaded {
		request := value.(*flyPacket)
		<-request.resultBack
		return request.result, request.err
	}
	if value, loaded := m.pendingResp.LoadOrStore(pkt.Id, newFlyRequest()); !loaded {
		request := value.(*flyPacket)
		packet := packetCoder.marshal(pkt)
		m.SendMessage(packet)
		beAcked := false
		timeout := time.After(time.Second * time.Duration(m.options.RetryTimeout))
		for {
			select {
			case <-request.resultBack:
				return request.result, request.err
			case <-request.ackBack:
				beAcked = true
			case <-time.After(time.Millisecond * time.Duration(m.options.RetryInterval*1000)):
				if !beAcked {
					m.SendMessage(packet)
				}
			case <-timeout:
				request.err = errors.New("超时未接收到RequestAck或者Response回复")
				if _, loaded = m.pendingResp.LoadAndDelete(pkt.Id); loaded {
					close(request.resultBack)
				}
				return nil, request.err
			}
		}
	} else {
		request := value.(*flyPacket)
		<-request.resultBack
		return request.result, request.err
	}
}

// 发送packet,重复发直到收到packet ack或者超时
func (m *Client) SendPacketWithRetry(pkt INetPacket) error {
	if value, loaded := m.pendingAck.Load(pkt.GetId()); loaded {
		notify := value.(*flyPacket)
		<-notify.ackBack
		return notify.err
	}
	if value, loaded := m.pendingAck.LoadOrStore(pkt.GetId(), newFlyNotify()); !loaded {
		notify := value.(*flyPacket)
		packet := packetCoder.marshal(pkt)
		m.SendMessage(packet)
		timeout := time.After(time.Second * time.Duration(m.options.RetryTimeout))
		for {
			select {
			case <-notify.ackBack:
				return notify.err
			case <-time.After(time.Millisecond * time.Duration(m.options.RetryInterval*1000)):
				m.SendMessage(packet)
			case <-timeout:
				notify.err = errors.New("超时未接收到ResponseAck回复")
				if _, loaded = m.pendingAck.LoadAndDelete(pkt.GetId()); loaded {
					close(notify.ackBack)
				}
				return notify.err
			}
		}
	} else {
		notify := value.(*flyPacket)
		<-notify.ackBack
		return notify.err
	}
}

func (m *Client) Publish(topic string, data interface{}) {
	m.SendPacket(&PublishPacket{Topic: topic, Params: data})
}

func (m *Client) PublishWithRetry(topic string, data interface{}) {
	m.SendPacketWithRetry(&PublishPacket{Id: uuid.New().String(), Topic: topic, Params: data})
}

func (m *Client) Subscribe(topic string, callback func(data *PublishRawPacket, from *Client)) error {
	id := uuid.New().String()
	m.SubTopic(NewTopicListener(topic, callback))
	return m.SendPacketWithRetry(&SubscribePacket{Id: id, Topic: topic})
}

func (m *Client) SubscribeAttributes(topic string, attributes []string, callback func(data *PublishRawPacket, from *Client)) error {
	id := uuid.New().String()
	m.SubTopic(NewTopicListener(topic, callback))
	return m.SendPacketWithRetry(&SubscribePacket{Id: id, Topic: topic, Attributes: attributes})
}

func (m *Client) Unsubscribe(topic string) error {
	id := uuid.New().String()
	m.UnsubTopic(topic)
	return m.SendPacketWithRetry(&UnSubscribePacket{Id: id, Topic: topic})
}

// 发送Request,重复发直到收到RequestAck或收到Request执行结果或超时(Request_timeout)
func (m *Client) StreamRequest(method string, params interface{}, execute func(stream *Stream) error) {
	stream := newStream(uuid.New().String(), m)
	m.handlingStream.Store(stream.Id, stream)
	stream.Client.RequestWithRetry(method, params)
	err := execute(stream)
	if _, ok := m.handlingStream.LoadAndDelete(stream.Id); !ok { //已被处理close
		return
	}
	closePkt := &StreamClosePacket{StreamId: stream.Id}
	if err != nil {
		logger.Error("stream执行出错", zap.Error(err))
		closePkt.Error = err.Error()
	}
	err = m.SendPacketDirect(closePkt)
	if err != nil {
		logger.Error("发送StreamClosePacket出错", zap.Error(err))
	}
}

func (m *Client) CallClient(params *CallClientParams) (interface{}, error) {
	return m.RequestWithRetryByPacket(&RequestPacket{Id: uuid.New().String(), Method: "call_client", Params: params})
}

func (m *Client) Login(params *LoginParams) (string, error) {
	pkt := &RequestPacket{Id: uuid.New().String(), Method: "login", Params: params}
	sessionId, err := m.RequestWithRetryByPacket(pkt)
	if err != nil {
		return "", err
	}
	return sessionId.(string), nil
}

func (m *Client) SendMessage(message []byte) error {
	return m.conn.Load().(IConn).SendMessage(message)
}

func (m *Client) SendMessageDirect(message []byte) error {
	return m.conn.Load().(IConn).SendMessageDirect(message)
}

func (m *Client) SendPacket(packet INetPacket) error {
	return m.SendMessage(packetCoder.marshal(packet))
}

func (m *Client) SendPacketDirect(packet INetPacket) error {
	return m.SendMessageDirect(packetCoder.marshal(packet))
}

func (m *Client) Close() {
	if m.isClosed.Load() {
		return
	}
	m.isClosed.Store(true)
	logger.Info("客户端断连", zap.String("clientId", m.ClientId))
	m.OnDisconnect.RiseEvent(nil)
	m.ClearAllSubTopics()
	m.conn.Load().(IConn).Close()
}

func (m *Client) IsClosed() bool {
	return m.isClosed.Load()
}

type flyPacket struct {
	resultBack chan struct{}
	ackBack    chan struct{}
	result     interface{}
	err        error
}

func newFlyRequest() *flyPacket {
	return &flyPacket{
		resultBack: make(chan struct{}, 1),
		ackBack:    make(chan struct{}, 1),
	}
}

func newFlyNotify() *flyPacket {
	return &flyPacket{
		ackBack: make(chan struct{}, 1),
	}
}

type RegisterServiceParams struct {
	Method []string
}

type CallClientParams struct {
	ClientId string
	Message  *RequestRawPacket
}

type LoginParams struct {
	Token    *string `json:"token"`
	BucketId *int64  `json:"bucket_id"`
	ClientId string  `json:"client_id"`
}

type streamHandler func(first *Packet, stream *Stream) error

func (s streamHandler) execute(first *Packet, stream *Stream) error {
	var executeErr error
	func() {
		defer func() {
			if err := recover(); err != nil {
				executeErr = err.(error)
			}
		}()
		executeErr = s(first, stream)
	}()
	return executeErr
}

type requestHandler func(pkt *Packet) (interface{}, error)

func (r requestHandler) execute(pkt *Packet) (interface{}, error) {
	var executeErr error
	var result interface{}
	func() {
		defer func() {
			if err := recover(); err != nil {
				executeErr = err.(error)
				fmt.Println("rpc execute exception:", err)
			}
		}()
		result, executeErr = r(pkt)
	}()
	return result, executeErr
}
