package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
)

type HubOptions struct {
	HeartbeatTimeout float64
	RetryTimeout     float64
	RetryInterval    float64
}

type Hub struct {
	//sessionId->*sessionData
	temptCacheSession sync.Map
	//clientId->*NetClient
	clients sync.Map
	//id->*NetBucket
	buckets sync.Map
	*Group
	*handlerMgr
	options *HubOptions
}

func New(options *HubOptions) *Hub {
	r := &Hub{
		Group:      newGroup(-1, nil),
		handlerMgr: &handlerMgr{},
		options:    options,
	}

	r.RegisterRequestHandler("login", func(req *RequestPacket, client *Client) (interface{}, error) {
		var params LoginParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			return "失败", err
		}
		logger.Info(fmt.Sprintf("[%v]设置clientId为[%v]", client.ClientId, params.ClientId))
		client.ClientId = params.ClientId
		client.GroupId = params.BucketId
		return "成功", nil
	})

	r.RegisterRequestHandler(EXCHANGE_SECRET, func(req *RequestPacket, client *Client) (interface{}, error) {
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
		return pubkey, nil
	})

	return r
}

type HubServerOptions struct {
	Codec      ICodec
	Crypto     *Crypto
	NeedLogin  bool
	ServerOpts []func(opt *ServerOptions)
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
	//udp Server
	udp := NewUdpServer(addr)
	var addrToClient sync.Map
	udp.OnReceiveMessage = func(rawData []byte, addr net.Addr, lis *udpListener) {
		var client *Client
		if value, loaded := addrToClient.Load(addr); loaded {
			client = value.(*Client)
		} else {
			client = newClient(&fakeUdpConn{Addr: addr, listener: lis}, &ClientOptions{
				HeartbeatTimeout: n.options.HeartbeatTimeout,
				WaitTimeout:      n.options.RetryTimeout,
				RetryInterval:    n.options.RetryInterval,
				PacketCodec:      options.Codec,
				Crypto:           options.Crypto,
				NeedLogin:        options.NeedLogin,
			})
			client.changeState(CONNECTED)
			client.handlerMgr = n.handlerMgr

			value, _ = addrToClient.LoadOrStore(addr, client)
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
	//tcp Server
	tcp := NewTcpServer(addr, options.ServerOpts...)
	tcp.OnClientConnect = func(conn *TcpConn) {
		client := newClient(conn, &ClientOptions{
			HeartbeatTimeout: n.options.HeartbeatTimeout,
			WaitTimeout:      n.options.RetryTimeout,
			RetryInterval:    n.options.RetryInterval,
			PacketCodec:      options.Codec,
			Crypto:           options.Crypto,
			NeedLogin:        options.NeedLogin,
		})
		client.OnLogin.AddEventListener(func(data interface{}) {
			n.onClientLogin(client)
		})
		client.changeState(CONNECTED)
		client.handlerMgr = n.handlerMgr

		conn.ListenToOnMessage(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.ListenToOnDisconnect(func(i interface{}) {
			client.ClearAllSubTopics()
			client.changeState(DISCONNECT)
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
	//websocket Server
	ws := NewWebsocketServer(addr, options.ServerOpts...)
	ws.OnClientConnect = func(conn *WebsocketConn) {
		client := newClient(conn, &ClientOptions{
			HeartbeatTimeout: n.options.HeartbeatTimeout,
			WaitTimeout:      n.options.RetryTimeout,
			RetryInterval:    n.options.RetryInterval,
			PacketCodec:      options.Codec,
			Crypto:           options.Crypto,
			NeedLogin:        options.NeedLogin,
		})
		client.OnLogin.AddEventListener(func(data interface{}) {
			n.onClientLogin(client)
		})
		client.changeState(CONNECTED)
		client.handlerMgr = n.handlerMgr

		conn.ListenToOnMessage(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.ListenToOnDisconnect(func(i interface{}) {
			client.ClearAllSubTopics()
			client.changeState(DISCONNECT)
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

func (n *Hub) AddClient(client *Client) {
	logger.Info("hub新增加客户端", zap.String("clientId", client.ClientId))
	n.clients.Store(client.ClientId, client)
}

func (n *Hub) RemoveClient(clientId string) {
	logger.Info("hub移除客户端", zap.String("clientId", clientId))
	n.clients.Delete(clientId)
}

func (n *Hub) onClientLogin(new *Client) {
	if client, loaded := n.clients.LoadAndDelete(new.ClientId); loaded {
		logger.Error(fmt.Sprintf("clientId【%v】已连接,关闭旧连接", new.ClientId))
		client.(*Client).Dispose()
	}
	n.AddClient(new)
	if new.GroupId != nil {
		group := n.FindOrCreateGroup(*new.GroupId)
		group.AddClient(new)
	} else {
		n.Group.AddClient(new)
	}

	new.OnDispose.AddEventListener(func(data interface{}) {
		n.RemoveClient(new.ClientId)
		if new.GroupId != nil {
			group := n.FindOrCreateGroup(*new.GroupId)
			group.RemoveClient(new.ClientId)
		} else {
			n.Group.RemoveClient(new.ClientId)
		}
	})
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
