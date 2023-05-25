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
	//等待Response
	WaitTimeout float64
	//等待回复超时重试时间间隔
	RetryInterval float64
	//编码解码器
	Codec ICodec
	//加密
	Crypto *Crypto
}

type Client struct {
	ctx                context.Context
	cancel             context.CancelFunc
	ClientId           string
	Group              *Group
	GroupId            *int64
	conn               IConn
	OnLogin            *EventTarget
	OnDispose          *EventTarget
	OnHeartbeatTimeout *EventTarget
	OnPingHandler      func(pkt *PingPacket)
	OnPongHandler      func(pkt *PongPacket)
	isClosed           atomic.Bool

	lastPktTime   atomic.Value
	rxQueue       chan *Packet
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
var RxQueueLen = 100

// 客户端发布队列大小
var ClientPubChLen = 1000

func newClient(conn IConn, opts *ClientOptions) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	se := &Client{
		ctx:           ctx,
		cancel:        cancel,
		ClientId:      uuid.NewString(),
		conn:          conn,
		OnLogin:       newEventTarget(),
		OnDispose:     newEventTarget(),
		lastPktTime:   atomic.Value{},
		packetHandler: map[PacketTypeCode]func(pkt *Packet){},
		rxQueue:       make(chan *Packet, RxQueueLen),
		handlerMgr:    &handlerMgr{},
		PubSub:        newPubSub(ctx, ClientPubChLen),
		options:       opts,
	}
	se.lastPktTime.Store(time.Now())
	//包处理
	se.packetHandler[REQUEST_PACKET] = se.onReceiveRequest
	se.packetHandler[ACK_PACKET] = se.onReceiveAck
	se.packetHandler[RESPONSE_PACKET] = se.onReceiveResponse
	se.packetHandler[PUBLISH_PACKET] = se.onReceivePublish
	se.packetHandler[SUBSCRIBE_PACKET] = se.onReceiveSubscribe
	se.packetHandler[UNSUBSCRIBE_PACKET] = se.onReceiveSubscribe
	se.packetHandler[BROADCAST_PACKET] = se.onReceiveBroadcast
	se.packetHandler[STREAM_REQUEST_PACKET] = se.onReceiveStreamRequest
	se.packetHandler[STREAM_RESPONSE_PACKET] = se.onReceiveStreamResponse
	se.packetHandler[STREAM_CLOSE_PACKET] = se.onReceiveCloseStream
	se.packetHandler[PING_PACKET] = func(pkt *Packet) {
		if se.OnPingHandler != nil {
			se.OnPingHandler(pkt.PacketContent.(*PingPacket))
		}
		var content = PongPacket(*pkt.PacketContent.(*PingPacket))
		se.SendPacket(&content)
	}
	se.packetHandler[PONG_PACKET] = func(pkt *Packet) {
		if se.OnPongHandler != nil {
			se.OnPongHandler(pkt.PacketContent.(*PongPacket))
		}
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case pkt := <-se.rxQueue:
				atomic.AddInt64(&Enqueue, -1)
				atomic.AddInt64(&EnConsume, 1)
				handler, ok := se.packetHandler[pkt.PacketType]
				if !ok {
					logger.Error("rpc通信包找不到handler", zap.Any("packet", string(pkt.RawData)))
					return
				}
				handler(pkt)
			case <-ticker.C:
				if time.Now().Sub(se.lastPktTime.Load().(time.Time)) > time.Millisecond*time.Duration(se.options.HeartbeatTimeout*1000*0.3) {
					//logger.Warn(fmt.Sprintf("客户端【%v】无数据,发送PING.", m.ClientId))
					var ping PingPacket = "PING"
					se.SendPacket(&ping)
				}
				if time.Now().Sub(se.lastPktTime.Load().(time.Time)) > time.Millisecond*time.Duration(se.options.HeartbeatTimeout*1000) {
					logger.Error(fmt.Sprintf("客户端【%v】超时无数据,断开连接.", se.ClientId))
					se.Close()
				}
			}
		}
	}()

	return se
}

func (m *Client) GetProperty(property string) (interface{}, bool) {
	return m.properties.Load(property)
}

func (m *Client) SetProperty(property string, value interface{}) {
	m.properties.Store(property, value)
}

func (m *Client) receiveMessage(rawData []byte) {
	payload, err := m.options.Codec.Unmarshal(rawData)
	if err != nil {
		logger.Error("net通信包解析出错", zap.Any("rawData", string(rawData)))
		return
	}
	pkt := &Packet{
		RawData:       rawData,
		PacketType:    payload.(INetPacket).TypeCode(),
		PacketContent: payload.(INetPacket),
		Client:        nil,
	}
	pkt.Client = m
	atomic.AddInt64(&Enqueue, 1)
	m.lastPktTime.Store(time.Now())
	m.rxQueue <- pkt
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
			handler(request, pkt.Client)
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
	if m.Group != nil {
		request.Topic = fmt.Sprintf("%v/%v", m.ClientId, request.Topic)
		request.ClientId = m.ClientId
		m.Group.PubTopic(request, m)
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
	if m.Group != nil {
		m.Group.Broadcast(pkt.RawData)
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
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
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
		packet := defaultCodec.Marshal(pkt)
		m.SendMessage(packet)
		beAcked := false
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
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
		packet := defaultCodec.Marshal(pkt)
		m.SendMessage(packet)
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
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
	return m.conn.SendMessage(message)
}

func (m *Client) SendMessageDirect(message []byte) error {
	return m.conn.SendMessageDirect(message)
}

func (m *Client) SendPacket(packet INetPacket) error {
	return m.SendMessage(defaultCodec.Marshal(packet))
}

func (m *Client) SendPacketDirect(packet INetPacket) error {
	return m.SendMessageDirect(defaultCodec.Marshal(packet))
}

func (m *Client) Close() {
	if m.isClosed.Load() {
		return
	}
	m.isClosed.Store(true)
	logger.Info("客户端断连", zap.String("clientId", m.ClientId))
	m.OnDispose.RiseEvent(nil)
	m.ClearAllSubTopics()
	m.conn.Close()
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

type requestHandler func(req *RequestRawPacket, client *Client) (interface{}, error)

func (r requestHandler) execute(req *RequestRawPacket, client *Client) (interface{}, error) {
	var executeErr error
	var result interface{}
	func() {
		defer func() {
			if err := recover(); err != nil {
				executeErr = err.(error)
				fmt.Println("rpc execute exception:", err)
			}
		}()
		result, executeErr = r(req, client)
	}()
	return result, executeErr
}
