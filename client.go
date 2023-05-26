package nethub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log"
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
	//整包的编码解码器
	PacketCodec ICodec
	//加密
	Crypto *Crypto
	//登录
	NeedLogin bool
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
	if opts.PacketCodec == nil {
		opts.PacketCodec = defaultCodec
	}
	cli := &Client{
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
	cli.lastPktTime.Store(time.Now())
	//包处理
	cli.packetHandler[REQUEST_PACKET] = cli.onReceiveRequest
	cli.packetHandler[ACK_PACKET] = cli.onReceiveAck
	cli.packetHandler[RESPONSE_PACKET] = cli.onReceiveResponse
	cli.packetHandler[PUBLISH_PACKET] = cli.onReceivePublish
	cli.packetHandler[SUBSCRIBE_PACKET] = cli.onReceiveSubscribe
	cli.packetHandler[UNSUBSCRIBE_PACKET] = cli.onReceiveSubscribe
	cli.packetHandler[BROADCAST_PACKET] = cli.onReceiveBroadcast
	cli.packetHandler[STREAM_REQUEST_PACKET] = cli.onReceiveStreamRequest
	cli.packetHandler[STREAM_RESPONSE_PACKET] = cli.onReceiveStreamResponse
	cli.packetHandler[STREAM_CLOSE_PACKET] = cli.onReceiveCloseStream
	cli.packetHandler[PING_PACKET] = func(pkt *Packet) {
		if cli.OnPingHandler != nil {
			cli.OnPingHandler(pkt.PacketContent.(*PingPacket))
		}
		var content = PongPacket(*pkt.PacketContent.(*PingPacket))
		cli.SendPacket(&content)
	}
	cli.packetHandler[PONG_PACKET] = func(pkt *Packet) {
		if cli.OnPongHandler != nil {
			cli.OnPongHandler(pkt.PacketContent.(*PongPacket))
		}
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-cli.ctx.Done():
				return
			case pkt := <-cli.rxQueue:
				atomic.AddInt64(&Enqueue, -1)
				atomic.AddInt64(&EnConsume, 1)
				handler, ok := cli.packetHandler[pkt.PacketType]
				if !ok {
					logger.Error("rpc通信包找不到handler", zap.Any("packet", string(pkt.RawData)))
					return
				}
				handler(pkt)
			case <-ticker.C:
				if time.Now().Sub(cli.lastPktTime.Load().(time.Time)) > time.Millisecond*time.Duration(cli.options.HeartbeatTimeout*1000*0.3) {
					//logger.Warn(fmt.Sprintf("客户端【%v】无数据,发送PING.", m.ClientId))
					var ping PingPacket = "PING"
					cli.SendPacket(&ping)
				}
				if time.Now().Sub(cli.lastPktTime.Load().(time.Time)) > time.Millisecond*time.Duration(cli.options.HeartbeatTimeout*1000) {
					logger.Error(fmt.Sprintf("客户端【%v】超时无数据,断开连接.", cli.ClientId))
					cli.Close()
				}
			}
		}
	}()

	return cli
}

func (m *Client) GetProperty(property string) (interface{}, bool) {
	return m.properties.Load(property)
}

func (m *Client) SetProperty(property string, value interface{}) {
	m.properties.Store(property, value)
}

func (m *Client) receiveMessage(rawData []byte) {
	log.Println("recv:", string(rawData))
	var needExchangeSecret = false
	crypto := m.options.Crypto
	if crypto != nil {
		switch crypto.state {
		case 2:
			crypto.Decode(rawData, rawData)
		case 1:
			crypto.Decode(rawData, rawData)
			crypto.state = 2
		case 0:
			needExchangeSecret = true
		}
	}
	payload, err := m.options.PacketCodec.Unmarshal(rawData)
	if err != nil {
		logger.Error("net通信包解析出错", zap.Any("rawData", string(rawData)))
		return
	}
	if needExchangeSecret && (payload.TypeCode() != REQUEST_PACKET || payload.(*RequestPacket).Method != "exchange_secret") {
		logger.Warn("client还未交互密钥,无效通信包", zap.Any("rawData", string(rawData)))
		return
	}
	if m.options.NeedLogin && (payload.TypeCode() != REQUEST_PACKET || payload.(*RequestPacket).Method != "login") {
		logger.Warn("client还未登录,无效通信包", zap.Any("rawData", string(rawData)))
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
	request := pkt.PacketContent.(*RequestPacket)
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
					result, err := handler.execute(request, m)
					if err != nil {
						logger.Error("request执行出错", zap.Error(err))
						resp.Error = err.Error()
					} else {
						resp.Result = result
					}
				}
				err := m.SendPacketWithRetry(resp)
				if err != nil {
					logger.Error("发送response出错", zap.String("clientId", m.ClientId), zap.String("From", m.conn.RemoteAddr().String()), zap.Error(err))
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
	request := pkt.PacketContent.(*PublishPacket)
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
	m.SubTopic(NewTopicListener(request.Topic, func(pkt *PublishPacket, from *Client) {
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
			pkt = &PublishPacket{Id: pkt.Id, Params: params, Topic: pkt.Topic, ClientId: pkt.ClientId}
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
func (m *Client) Request(method string, params []byte) (interface{}, error) {
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
func (m *Client) RequestWithRetry(method string, params []byte) (interface{}, error) {
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

func (m *Client) Publish(topic string, data []byte) {
	m.SendPacket(&PublishPacket{Topic: topic, Params: data})
}

func (m *Client) PublishWithRetry(topic string, data []byte) {
	m.SendPacketWithRetry(&PublishPacket{Id: uuid.New().String(), Topic: topic, Params: data})
}

func (m *Client) Subscribe(topic string, callback func(data *PublishPacket, from *Client)) error {
	id := uuid.New().String()
	m.SubTopic(NewTopicListener(topic, callback))
	return m.SendPacketWithRetry(&SubscribePacket{Id: id, Topic: topic})
}

func (m *Client) SubscribeAttributes(topic string, attributes []string, callback func(data *PublishPacket, from *Client)) error {
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
func (m *Client) StreamRequest(method string, params []byte, execute func(stream *Stream) error) {
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

func (m *Client) SendMessage(message []byte) error {
	log.Println("send:", string(message))
	if m.options.Crypto != nil && m.options.Crypto.state == 2 {
		m.options.Crypto.Encode(message, message)
	}
	return m.conn.SendMessage(message)
}

func (m *Client) SendMessageDirect(message []byte) error {
	log.Println("send:", string(message))
	if m.options.Crypto != nil && m.options.Crypto.state == 2 {
		m.options.Crypto.Encode(message, message)
	}
	return m.conn.SendMessageDirect(message)
}

func (m *Client) SendPacket(packet INetPacket) error {
	return m.SendMessage(defaultCodec.Marshal(packet))
}

func (m *Client) SendPacketDirect(packet INetPacket) error {
	return m.SendMessageDirect(defaultCodec.Marshal(packet))
}

// 客户端进行登录
func (m *Client) Login(params *LoginParams) error {
	m.ClientId = params.ClientId
	m.GroupId = params.BucketId
	data, _ := json.Marshal(params)
	pkt := &RequestPacket{Id: uuid.New().String(), Method: "login", Params: data}
	_, err := m.RequestWithRetryByPacket(pkt)
	if err == nil {
		log.Println("客户端登录成功", zap.String("clientId", m.ClientId))
		m.OnLogin.RiseEvent(nil)
	}
	return err
}

func (m *Client) Close() {
	if m.isClosed.Load() {
		return
	}
	m.isClosed.Store(true)
	m.cancel()
	m.ClearAllSubTopics()
	m.conn.Close()
	logger.Info("客户端断连", zap.String("clientId", m.ClientId))
	m.OnDispose.RiseEvent(nil)
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

type LoginParams struct {
	BucketId *int64 `json:"bucket_id"`
	ClientId string `json:"client_id"`
}

type ExchangeSecretParams struct {
	PubKey []byte `json:"pub_key"`
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

type requestHandler func(req *RequestPacket, from *Client) (interface{}, error)

func (r requestHandler) execute(req *RequestPacket, client *Client) (interface{}, error) {
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
