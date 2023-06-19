package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net"
	"strconv"
	"sync"
	"time"
)

type HubOptions struct {
	//超时断线,默认10秒
	DisconnectTimeout float64
	//定时发送心跳包, 0 不发送心跳包
	HeartbeatInterval float64
	//等待心跳包超时, 0 不校验超时
	HeartbeatTimeout float64
	//等待Response
	WaitTimeout float64
	//等待回复超时重试时间间隔
	RetryInterval float64
	handlerMgr    *HandlerMgr
}

type Hub struct {
	//sessionId->*sessionData
	temptCacheSession sync.Map
	//clientId->*NetClient
	clients sync.Map
	//id->*NetBucket
	buckets sync.Map
	*Group
	options *HubOptions
}

func New(options *HubOptions) *Hub {
	if options.handlerMgr == nil {
		options.handlerMgr = &HandlerMgr{}
	}

	r := &Hub{
		Group:   newGroup(-1, nil),
		options: options,
	}
	r.options.handlerMgr.RegisterRequestHandler("login", func(req *RequestPacket, client *Client) ([]byte, error) {
		var params LoginParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			return []byte("失败"), err
		}
		logger.Info(fmt.Sprintf("[%v]设置clientId为[%v]", client.ClientId, params.ClientId))
		client.ClientId = params.ClientId
		if params.BucketId != nil {
			client.GroupId = *params.BucketId
		}

		result, _ := json.Marshal("成功")
		return result, nil
	})

	r.options.handlerMgr.RegisterRequestHandler(EXCHANGE_SECRET, func(req *RequestPacket, client *Client) ([]byte, error) {
		var params ExchangeSecretParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, fmt.Errorf("params unmarshal err:%v", err.Error())
		}
		if client.options.Crypto == nil {
			return nil, errors.New("未启用加密通信")
		}
		pubkey, err := client.options.Crypto.ComputeSecret(params.PubKey)
		if err != nil {
			return nil, fmt.Errorf("交换密钥失败,secret出错:%v", err.Error())
		}

		result, _ := json.Marshal(pubkey)
		return result, nil
	})

	return r
}

type HubServerOptions struct {
	Codec      ICodec
	Crypto     *Crypto
	NeedLogin  bool
	ServerOpts []func(opt *ServerOptions)
	handlerMgr *HandlerMgr
}

type HubServerOption func(opts *HubServerOptions)

func WithCodec(codec ICodec) HubServerOption {
	return func(opts *HubServerOptions) { opts.Codec = codec }
}

func WithCrypto(crypto *Crypto) HubServerOption {
	return func(opts *HubServerOptions) { opts.Crypto = crypto }
}

func WithNeedLogin(needLogin bool) HubServerOption {
	return func(opts *HubServerOptions) { opts.NeedLogin = needLogin }
}
func WithHandlerMgr(mgr *HandlerMgr) HubServerOption {
	return func(opts *HubServerOptions) { opts.handlerMgr = mgr }
}

func WithServerOpts(serOpts ...func(opt *ServerOptions)) HubServerOption {
	return func(opts *HubServerOptions) { opts.ServerOpts = serOpts }
}

func (n *Hub) ListenAndServeUdp(addr string, listenerCount int, opts ...HubServerOption) {
	var options HubServerOptions
	for _, opt := range opts {
		opt(&options)
	}
	if options.Codec == nil {
		options.Codec = defaultCodec
	}
	if options.handlerMgr == nil {
		options.handlerMgr = n.options.handlerMgr
	}
	//udp Server
	udp := NewUdpServer(addr)
	var addrToClient sync.Map
	udp.OnReceiveMessage = func(rawData []byte, addr net.Addr, lis *udpListener) {
		var client *Client
		if value, loaded := addrToClient.Load(addr.String()); loaded {
			client = value.(*Client)
		} else {
			client = newClient(&fakeUdpConn{Addr: addr, listener: lis}, &ClientOptions{
				DisconnectTimeout: n.options.DisconnectTimeout,
				HeartbeatTimeout:  n.options.HeartbeatTimeout,
				HeartbeatInterval: n.options.HeartbeatInterval,
				WaitTimeout:       n.options.WaitTimeout,
				RetryInterval:     n.options.RetryInterval,
				PacketCodec:       options.Codec,
				Crypto:            options.Crypto,
				NeedLogin:         options.NeedLogin,
			})
			//client.hub = n
			client.HandlerMgr = n.options.handlerMgr
			if client.BeReady() {
				n.AddClient(client)
			}
			client.OnReady.AddEventListener(func(data interface{}) {
				n.AddClient(client)
			})

			client.OnDispose.AddEventListener(func(data interface{}) {
				addrToClient.Delete(addr)
			})
			value, _ = addrToClient.LoadOrStore(addr.String(), client)
			client = value.(*Client)
		}
		client.receiveMessage(rawData)
	}
	udp.ListenAndServe(listenerCount)
}

func (n *Hub) ListenAndServeTcp(addr string, listenerCount int, opts ...HubServerOption) *TcpServer {
	var options HubServerOptions
	for _, opt := range opts {
		opt(&options)
	}
	if options.Codec == nil {
		options.Codec = defaultCodec
	}
	if options.handlerMgr == nil {
		options.handlerMgr = n.options.handlerMgr
	}
	//tcp Server
	tcp := NewTcpServer(addr, options.ServerOpts...)
	tcp.OnClientConnect = func(conn *TcpConn) {
		client := newClient(conn, &ClientOptions{
			DisconnectTimeout: n.options.DisconnectTimeout,
			HeartbeatTimeout:  n.options.HeartbeatTimeout,
			HeartbeatInterval: n.options.HeartbeatInterval,
			WaitTimeout:       n.options.WaitTimeout,
			RetryInterval:     n.options.RetryInterval,
			PacketCodec:       options.Codec,
			Crypto:            options.Crypto,
			NeedLogin:         options.NeedLogin,
		})
		//client.hub = n
		client.HandlerMgr = options.handlerMgr
		if client.BeReady() {
			n.AddClient(client)
		}
		client.OnReady.AddEventListener(func(data interface{}) {
			n.AddClient(client)
		})

		conn.ListenToOnMessage(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.ListenToOnDisconnect(func(i interface{}) {
			client.ClearAllSubTopics()
			client.disconnectTime = time.Now()
		})
	}
	go tcp.ListenAndServe(listenerCount)
	return tcp
}

func (n *Hub) ListenAndServeWebsocket(addr string, opts ...HubServerOption) *WebsocketServer {
	var options HubServerOptions
	for _, opt := range opts {
		opt(&options)
	}
	if options.Codec == nil {
		options.Codec = defaultCodec
	}
	if options.handlerMgr == nil {
		options.handlerMgr = n.options.handlerMgr
	}
	//websocket Server
	ws := NewWebsocketServer(addr, options.ServerOpts...)
	ws.OnClientConnect = func(conn *WebsocketConn) {
		cliOpts := &ClientOptions{
			DisconnectTimeout: n.options.DisconnectTimeout,
			HeartbeatTimeout:  n.options.HeartbeatTimeout,
			HeartbeatInterval: n.options.HeartbeatInterval,
			WaitTimeout:       n.options.WaitTimeout,
			RetryInterval:     n.options.RetryInterval,
			PacketCodec:       options.Codec,
			Crypto:            options.Crypto,
			NeedLogin:         options.NeedLogin,
		}
		cliOpts.ClientId = conn.UrlParams["client_id"]
		groupIdStr := conn.UrlParams["bucket_id"]
		if groupIdStr != "" {
			groupId, _ := strconv.ParseInt(groupIdStr, 10, 64)
			cliOpts.GroupId = groupId
		}
		client := newClient(conn, cliOpts)
		//client.hub = n
		client.HandlerMgr = options.handlerMgr
		if client.BeReady() {
			n.AddClient(client)
		}
		client.OnReady.AddEventListener(func(data interface{}) {
			n.AddClient(client)
		})

		conn.ListenToOnMessage(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.ListenToOnDisconnect(func(i interface{}) {
			client.ClearAllSubTopics()
			client.disconnectTime = time.Now()
		})
	}
	go ws.ListenAndServe()
	return ws
}

func (n *Hub) FindOrCreateGroup(id int64) *Group {
	if value, loaded := n.buckets.Load(id); loaded {
		return value.(*Group)
	}
	value, _ := n.buckets.LoadOrStore(id, newGroup(id, n.Group))
	return value.(*Group)
}

func (n *Hub) FindGroup(id int64) (*Group, bool) {
	if value, loaded := n.buckets.Load(id); loaded {
		return value.(*Group), true
	}
	return nil, false
}

// 外部httpserver暂存
func (n *Hub) TemptSaveSession(data SessionData) {
	n.temptCacheSession.Store(data.SessionId, data)
	go func() {
		time.Sleep(time.Minute * 2)
		n.temptCacheSession.Delete(data.SessionId)
	}()
}

func (n *Hub) FindClient(id string) (*Client, bool) {
	value, ok := n.clients.Load(id)
	if !ok {
		return nil, false
	}
	return value.(*Client), true
}

func (n *Hub) AddClient(new *Client) {
	if value, loaded := n.clients.Load(new.ClientId); loaded {
		if value.(*Client) != new {
			logger.Error(fmt.Sprintf("clientId【%v】已连接,关闭旧连接[%v]", new.ClientId, new.conn.RemoteAddr()))
			value.(*Client).Dispose()
			n.clients.Store(new.ClientId, new)

			if new.GroupId != 0 {
				group := n.FindOrCreateGroup(new.GroupId)
				group.AddClient(new)
			} else {
				n.Group.AddClient(new)
			}
			new.OnDispose.AddEventListener(func(data interface{}) {
				logger.Info("hub移除客户端", zap.String("clientId", new.ClientId))
				n.clients.Delete(new.ClientId)
			})
		}
	} else {
		logger.Info("hub新增加客户端", zap.String("clientId", new.ClientId), zap.String("协议", new.conn.Type()), zap.String("addr", new.conn.RemoteAddr().String()))
		n.clients.Store(new.ClientId, new)
		if new.GroupId != 0 {
			group := n.FindOrCreateGroup(new.GroupId)
			group.AddClient(new)
		} else {
			n.Group.AddClient(new)
		}
		new.OnDispose.AddEventListener(func(data interface{}) {
			logger.Info("hub移除客户端", zap.String("clientId", new.ClientId))
			n.clients.Delete(new.ClientId)
		})
	}
}

func (n *Hub) RegisterRequestHandler(method string, handler requestHandler) {
	n.options.handlerMgr.RegisterRequestHandler(method, handler)
}

func (n *Hub) RegisterStreamHandler(method string, handler streamHandler) {
	n.options.handlerMgr.RegisterStreamHandler(method, handler)
}

type SessionData struct {
	ClientId  string `json:"client_id"`
	NodeId    *int64 `json:"node_id"`
	SessionId string `json:"session_id,omitempty"`
}

type CheckLogin func(params LoginParams) error

type Packet struct {
	//原始pkt
	RawData []byte
	//pkt类型
	PacketType PacketTypeCode
	//pkt内容包
	PacketContent INetPacket
	Client        *Client
}
