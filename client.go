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
	//整包的编码解码器
	PacketCodec ICodec
	//加密
	Crypto *Crypto
	//登录
	NeedLogin bool
}

type clientState string

var (
	DISCONNECT       clientState = "DISCONNECT"
	CONNECTED        clientState = "CONNECTED"
	EXCHANGED_SECRET clientState = "EXCHANGED_SECRET"
	LOGINED          clientState = "LOGINED"
	DISPOSED         clientState = "DISPOSED"
)

var EXCHANGE_SECRET = "exchange_secret"
var LOGIN = "login"

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	//是否是客户端
	beClient           atomic.Bool
	ClientId           string
	Group              *Group
	GroupId            *int64
	conn               IConn
	OnReady            *EventTarget
	OnDispose          *EventTarget
	OnHeartbeatTimeout *EventTarget
	OnPingHandler      func(pkt *PingPacket)
	OnPongHandler      func(pkt *PongPacket)
	beDisposed         atomic.Bool
	state              atomic.Value
	beExchangedSecret  atomic.Bool
	beLogin            atomic.Bool

	rxQueue       chan *Packet
	lastRxTime    atomic.Value
	packetHandler map[PacketTypeCode]func(pkt *Packet)
	lastTxTime    atomic.Value

	*HandlerMgr
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
		beClient:      atomic.Bool{},
		state:         atomic.Value{},
		ClientId:      uuid.NewString(),
		conn:          conn,
		OnReady:       newEventTarget(),
		OnDispose:     newEventTarget(),
		lastRxTime:    atomic.Value{},
		packetHandler: map[PacketTypeCode]func(pkt *Packet){},
		rxQueue:       make(chan *Packet, RxQueueLen),
		HandlerMgr:    &HandlerMgr{},
		PubSub:        newPubSub(ctx, ClientPubChLen),
		options:       opts,
	}
	cli.state.Store(DISCONNECT)
	cli.beClient.Store(false)
	cli.lastRxTime.Store(time.Now())
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
	cli.packetHandler[HEARTBEAT_PACKET] = func(pkt *Packet) {}

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
				if cli.BeConnected() {
					//如果无发送包,定时发送心跳包
					if cli.lastTxTime.Load() == nil || time.Now().Sub(cli.lastTxTime.Load().(time.Time)) > time.Millisecond*time.Duration(cli.options.HeartbeatTimeout*1000*0.3) {
						var heartbeat HeartbeatPacket = ""
						cli.SendPacket(&heartbeat)
					}
				}

				if cli.beClient.Load() && cli.BeConnected() {
					if time.Now().Sub(cli.lastRxTime.Load().(time.Time)) > time.Millisecond*time.Duration(cli.options.HeartbeatTimeout*1000) {
						logger.Error(fmt.Sprintf("[%v]超时无数据,断开连接.", cli.ClientId))
						cli.conn.Close()
					}
				} else {
					if time.Now().Sub(cli.lastRxTime.Load().(time.Time)) > time.Millisecond*time.Duration(cli.options.HeartbeatTimeout*1000) {
						logger.Error(fmt.Sprintf("[%v]超时无数据,断开连接.", cli.ClientId))
						cli.Dispose()
					}
				}
			}
		}
	}()
	logger.Info(fmt.Sprintf("初始化client[%v]", cli.ClientId))
	return cli
}

func (m *Client) GetProperty(property string) (interface{}, bool) {
	return m.properties.Load(property)
}

func (m *Client) SetProperty(property string, value interface{}) {
	m.properties.Store(property, value)
}

func (m *Client) receiveMessage(rawData []byte) {
	payload, err := m.options.PacketCodec.Unmarshal(rawData, m.options.Crypto)
	if err != nil {
		logger.Error("net通信包解析出错", zap.Any("rawData", string(rawData)))
		return
	}
	pkt := &Packet{
		RawData:       rawData,
		PacketType:    payload.(INetPacket).TypeCode(),
		PacketContent: payload.(INetPacket),
		Client:        m,
	}
	if m.options.Crypto != nil && !m.beExchangedSecret.Load() {
		switch payload.TypeCode() {
		case REQUEST_PACKET:
			if payload.(*RequestPacket).Method != EXCHANGE_SECRET {
				logger.Warn("client还未交互密钥,丢弃消息！", zap.Any("packet", pkt))
				return
			}
		case ACK_PACKET, RESPONSE_PACKET:
		default:
			logger.Warn("client还未交互密钥,丢弃消息！", zap.Any("packet", pkt))
			return
		}
	}
	if m.options.NeedLogin && !m.beLogin.Load() {
		switch payload.TypeCode() {
		case REQUEST_PACKET:
			if payload.(*RequestPacket).Method != EXCHANGE_SECRET && payload.(*RequestPacket).Method != LOGIN {
				logger.Warn("client还未登录,丢弃消息！", zap.Any("packet", pkt))
				return
			}
		case ACK_PACKET, RESPONSE_PACKET:
		default:
			logger.Warn("client还未登录,丢弃消息！", zap.Any("packet", pkt))
			return
		}
	}
	atomic.AddInt64(&Enqueue, 1)
	m.lastRxTime.Store(time.Now())
	m.rxQueue <- pkt
}

func (m *Client) BeConnected() bool {
	if m.conn == nil {
		return false
	}
	return !m.conn.IsClosed()
}

func (m *Client) BeReady() bool {
	if m.conn.IsClosed() {
		return false
	}
	if m.options.Crypto != nil && !m.beExchangedSecret.Load() {
		return false
	}
	if m.options.NeedLogin && !m.beLogin.Load() {
		return false
	}
	return true
}

func (m *Client) onReceiveRequest(pkt *Packet) {
	request := pkt.PacketContent.(*RequestPacket)
	request.ClientId = m.ClientId
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
				switch request.Method {
				case EXCHANGE_SECRET:
					if resp.Error == "" && err == nil {
						m.beExchangedSecret.Store(true)
						if !m.options.NeedLogin {
							m.OnReady.RiseEvent(nil)
						}
					}
				case LOGIN:
					if resp.Error == "" && err == nil {
						m.beLogin.Store(true)
						m.OnReady.RiseEvent(nil)
					}
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
	request.ClientId = m.ClientId
	if request.Id != "" {
		m.SendPacket(&AckPacket{Id: request.Id})
	}
	if m.Group != nil {
		request.Topic = fmt.Sprintf("%v/%v", m.ClientId, request.Topic)
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
	request.ClientId = m.ClientId
	if m.Group != nil {
		m.Group.Broadcast(request)
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
		select {
		case <-m.ctx.Done():
			return nil, errors.New("client已close")
		case <-request.resultBack:
			return request.result, request.err
		}
		return request.result, request.err
	}
	if value, loaded := m.pendingResp.LoadOrStore(pkt.Id, newFlyRequest()); !loaded {
		request := value.(*flyPacket)
		m.SendPacket(pkt)
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
		for {
			select {
			case <-m.ctx.Done():
				return nil, errors.New("client已close")
			case <-request.resultBack:
				return request.result, request.err
			case <-timeout:
				request.err = fmt.Errorf("超时未接收到RequestAck或者Response回复,reqId=%v", pkt.GetId())
				close(request.resultBack)
				return nil, request.err
			}
		}
	} else {
		request := value.(*flyPacket)
		select {
		case <-m.ctx.Done():
			return nil, errors.New("client已close")
		case <-request.resultBack:
			return request.result, request.err
		}
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
		select {
		case <-m.ctx.Done():
			return nil, errors.New("client已close")
		case <-request.resultBack:
			return request.result, request.err
		}
	}
	if value, loaded := m.pendingResp.LoadOrStore(pkt.Id, newFlyRequest()); !loaded {
		request := value.(*flyPacket)
		m.SendPacket(pkt)
		beAcked := false
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
		for {
			select {
			case <-m.ctx.Done():
				return nil, errors.New("client已close")
			case <-request.resultBack:
				return request.result, request.err
			case <-request.ackBack:
				beAcked = true
			case <-time.After(time.Millisecond * time.Duration(m.options.RetryInterval*1000)):
				if !beAcked {
					m.SendPacket(pkt)
				}
			case <-timeout:
				request.err = fmt.Errorf("超时未接收到RequestAck或者Response回复,reqId=%v", pkt.GetId())
				if _, loaded = m.pendingResp.LoadAndDelete(pkt.Id); loaded {
					close(request.resultBack)
				}
				return nil, request.err
			}
		}
	} else {
		request := value.(*flyPacket)
		select {
		case <-m.ctx.Done():
			return nil, errors.New("client已close")
		case <-request.resultBack:
			return request.result, request.err
		}
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
		m.SendPacket(pkt)
		timeout := time.After(time.Second * time.Duration(m.options.WaitTimeout))
		for {
			select {
			case <-m.ctx.Done():
				return nil
			case <-notify.ackBack:
				return notify.err
			case <-time.After(time.Millisecond * time.Duration(m.options.RetryInterval*1000)):
				m.SendPacket(pkt)
			case <-timeout:
				notify.err = fmt.Errorf("超时未接收到ResponseAck[id=%v]回复", pkt.GetId())
				if _, loaded = m.pendingAck.LoadAndDelete(pkt.GetId()); loaded {
					close(notify.ackBack)
				}
				return notify.err
			}
		}
	} else {
		notify := value.(*flyPacket)
		select {
		case <-m.ctx.Done():
			return nil
		case <-notify.ackBack:
			return notify.err
		}
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
		logger.Error("stream执行出错", zap.String("streamId", stream.Id), zap.Error(err))
		closePkt.Error = err.Error()
	}
	err = m.SendPacketDirect(closePkt)
	if err != nil {
		logger.Error("发送StreamClosePacket出错", zap.String("streamId", stream.Id), zap.Error(err))
	}
}

func (m *Client) SendMessage(message []byte) error {
	m.lastTxTime.Store(time.Now())
	logger.Info(fmt.Sprintf("[%v]Send:%v", m.ClientId, string(message)))
	return m.conn.SendMessage(message)
}

func (m *Client) SendMessageDirect(message []byte) error {
	m.lastTxTime.Store(time.Now())
	logger.Info(fmt.Sprintf("[%v]Send:%v", m.ClientId, string(message)))
	return m.conn.SendMessageDirect(message)
}

func (m *Client) SendPacket(packet INetPacket) error {
	var data []byte
	if m.beExchangedSecret.Load() {
		data = m.options.PacketCodec.Marshal(packet, m.options.Crypto)
	} else {
		data = m.options.PacketCodec.Marshal(packet, nil)
	}
	return m.SendMessage(data)
}

func (m *Client) SendPacketDirect(packet INetPacket) error {
	var data []byte
	if m.beExchangedSecret.Load() {
		data = m.options.PacketCodec.Marshal(packet, m.options.Crypto)
	} else {
		data = m.options.PacketCodec.Marshal(packet, nil)
	}
	return m.SendMessageDirect(data)
}

// 客户端进行登录
func (m *Client) Login(params *LoginParams) error {
	if m.options.Crypto != nil {
		secretParams, _ := json.Marshal(&ExchangeSecretParams{PubKey: m.options.Crypto.Base64PubKey()})
		resp, err := m.Request(EXCHANGE_SECRET, secretParams)
		if err != nil {
			return err
		}
		_, err = m.options.Crypto.ComputeSecret(resp.(string))
		if err != nil {
			return err
		}
		m.beExchangedSecret.Store(true)
	}
	logger.Info(fmt.Sprintf("[%v]设置clientId为[%v]", m.ClientId, params.ClientId))
	m.ClientId = params.ClientId
	m.GroupId = params.BucketId
	data, _ := json.Marshal(params)
	_, err := m.Request("login", data)
	if err == nil {
		m.beLogin.Store(true)
		m.OnReady.RiseEvent(nil)
	}
	return err
}

func (m *Client) Dispose() {
	if m.beDisposed.Load() {
		return
	}
	m.beDisposed.Store(true)
	m.cancel()
	m.ClearAllSubTopics()
	logger.Info("客户端释放", zap.String("clientId", m.ClientId), zap.String("from", m.conn.RemoteAddr().String()))
	m.conn.Close()
	m.OnDispose.RiseEvent(nil)
}

type flyPacket struct {
	resultBack chan struct{}
	ackBack    chan struct{}
	result     []byte
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
	PubKey string `json:"pub_key"`
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

type requestHandler func(req *RequestPacket, from *Client) ([]byte, error)

func (r requestHandler) execute(req *RequestPacket, client *Client) ([]byte, error) {
	var executeErr error
	var result []byte
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
