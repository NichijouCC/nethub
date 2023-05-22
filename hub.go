package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
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
	//bucketId->*NetBucket
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

	r.RegisterRequestHandler("login", func(pkt *Packet) (interface{}, error) {
		return pkt.Client.sessionId, nil
	})

	r.RegisterRequestHandler("register_service", func(pkt *Packet) (interface{}, error) {
		var params RegisterServiceParams
		err := json.Unmarshal([]byte(pkt.PacketContent.(*RequestRawPacket).Params), &params)
		if err != nil {
			return nil, errors.New("命令json解析出错")
		}
		for _, s := range params.Method {
			r.RegisterRequestHandler(s, func(pkt *Packet) (interface{}, error) {
				req := pkt.PacketContent.(*RequestRawPacket)
				if req.Id == "" {
					pkt.Client.SendPacket(req)
					return "成功", nil
				} else {
					return pkt.Client.RequestWithRetryByPacket(&RequestPacket{
						Id:     req.Id,
						Method: req.Method,
						Params: req.Params,
					})
				}
			})
		}
		return "成功", nil
	})

	r.RegisterRequestHandler("call_client", func(pkt *Packet) (interface{}, error) {
		req := pkt.PacketContent.(*RequestRawPacket)
		var params CallClientParams
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("请求params解析出错")
		}
		client, oK := pkt.Client.Bucket.FindClient(params.ClientId)
		if !oK {
			return nil, fmt.Errorf("无法找到对方[id=%v]", params.ClientId)
		}
		if req.Id == "" {
			client.SendPacket(params.Message)
			return "已发送", nil
		} else {
			return client.RequestWithRetryByPacket(&RequestPacket{Id: req.Id, Method: params.Message.Method, Params: params.Message.Params})
		}
	})
	return r
}

func (n *Hub) ListenAndServeUdp(addr string, listenerCount int) {
	//udp Server
	udp := NewUdpServer(addr)
	var addrToClient sync.Map
	udp.OnReceiveMessage = func(rawData []byte, conn *udpConn) {
		pkt, err := packetCoder.unmarshal(rawData)
		if err != nil {
			logger.Error("net通信包解析出错", zap.Any("packet", string(rawData)))
			return
		}
		value, loaded := addrToClient.LoadOrStore(conn.Addr.String(), struct{}{})
		if !loaded {
			client := n.createClient(conn, &SessionData{
				ClientId:  uuid.New().String(),
				SessionId: uuid.New().String(),
			})
			addrToClient.Store(conn.Addr.String(), client)
			client.conn.Store(conn)
			client.receivePacket(pkt)
		} else {
			client := value.(*Client)
			client.conn.Store(conn)
			client.receivePacket(pkt)
		}
	}
	udp.ListenAndServe(listenerCount)
}

// 这个起的server会进行消息验证,需要自行构建一个httpServer提供登录接口,用于登录获得sessionId
func (n *Hub) ListenAndServeUdpWithAuth(addr string, listenerCount int) *UdpServer {
	//udp Server
	udp := NewUdpServer(addr)
	//sessionId->*NetClient
	var usingSession sync.Map
	udp.OnReceiveMessage = func(rawData []byte, conn *udpConn) {
		pkt, err := packetCoder.unmarshal(rawData)
		if err != nil {
			logger.Error("net通信包解析出错", zap.Any("packet", string(rawData)))
			return
		}
		if pkt.Util != "" {
			if value, loaded := usingSession.Load(pkt.Util); loaded {
				client := value.(*Client)
				client.conn.Store(conn)
				client.receivePacket(pkt)
			} else {
				if value, loaded = n.temptCacheSession.LoadAndDelete(pkt.Util); loaded {
					data := value.(*SessionData)
					client := n.createClient(conn, data)
					usingSession.Store(data.SessionId, client)
					client.conn.Store(conn)
					client.receivePacket(pkt)
				}
			}
		}
	}
	udp.ListenAndServe(listenerCount)
	return udp
}

func (n *Hub) ListenAndServeTcpWithAuth(addr string, listenerCount int, checkLogin CheckLogin) {
	tcp := NewTcpServer(addr, WithAuth(&authOptions{
		Timeout: time.Second,
		CheckFunc: func(first []byte, conn IConn) (interface{}, error) {
			pkt, err := packetCoder.unmarshal(first)
			if err != nil {
				logger.Error("net通信包解析出错", zap.Any("packet", string(first)))
				return nil, errors.New("认证失败")
			}
			if pkt.PacketType == REQUEST_PACKET && pkt.PacketContent.(*RequestRawPacket).Method == "login" {
				req := pkt.PacketContent.(*RequestRawPacket)
				var params LoginParams
				err = json.Unmarshal(req.Params, &params)
				if err != nil {
					conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Error: "认证失败"}))
					return nil, errors.New("认证失败")
				}
				err = checkLogin(params)
				if err != nil {
					conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Error: "认证失败"}))
					return nil, err
				}
				conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Result: "认证成功"}))
				//创建client
				data := &SessionData{
					ClientId:  params.ClientId,
					NodeId:    params.BucketId,
					SessionId: uuid.New().String(),
				}
				n.createClient(conn, data)
				return data, nil
			} else {
				return nil, errors.New("认证失败")
			}
		},
	}))
	go tcp.ListenAndServe(listenerCount)
}

func (n *Hub) ListenAndServeTcp(addr string, listenerCount int) *TcpServer {
	tcp := NewTcpServer(addr)
	tcp.OnClientConnect = func(conn *TcpConn) {
		//创建client
		n.createClient(conn, &SessionData{
			ClientId:  uuid.New().String(),
			SessionId: uuid.New().String(),
		})
	}
	go tcp.ListenAndServe(listenerCount)
	return tcp
}

func (n *Hub) ListenAndServeWebsocketWithAuth(addr string, checkLogin CheckLogin) *WebsocketServer {
	//websocket Server
	ws := NewWebsocketServer(addr, WithAuth(&authOptions{
		Timeout: time.Second,
		CheckFunc: func(loginInfo []byte, conn IConn) (interface{}, error) { //conn为nil
			var urlParams map[string]string
			err := json.Unmarshal(loginInfo, &urlParams)
			if err != nil {
				return nil, err
			}
			var params LoginParams
			if clientId, ok := urlParams["client_id"]; ok {
				params.ClientId = clientId
			} else {
				return nil, errors.New("认证失败")
			}
			if bucketId, ok := urlParams["bucket_id"]; ok {
				id, err := strconv.ParseInt(bucketId, 10, 64)
				if err != nil {
					return nil, errors.New("认证失败")
				}
				params.BucketId = &id
			}
			if token, ok := urlParams["token"]; ok {
				params.Token = &token
			}
			err = checkLogin(params)
			if err != nil {
				return nil, err
			}
			data := &SessionData{
				ClientId:  params.ClientId,
				NodeId:    params.BucketId,
				SessionId: uuid.New().String(),
			}
			return data, nil
		},
	}))
	ws.OnClientConnect = func(conn *WebsocketConn) {
		n.createClient(conn, conn.auth.(*SessionData))
	}
	go ws.ListenAndServe()
	return ws
}

func (n *Hub) ListenAndServeWebsocket(addr string) *WebsocketServer {
	//websocket Server
	ws := NewWebsocketServer(addr)
	ws.OnClientConnect = func(conn *WebsocketConn) {
		n.createClient(conn, &SessionData{
			ClientId:  uuid.New().String(),
			SessionId: uuid.New().String(),
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

type HttpResp struct {
	Code    int         `json:"code,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// 外部httpserver暂存
func (n *Hub) TemptSaveSession(data SessionData) {
	n.temptCacheSession.Store(data.SessionId, data)
	go func() {
		time.Sleep(time.Minute * 2)
		n.temptCacheSession.Delete(data.SessionId)
	}()
}

func (n *Hub) FindClient(clientId string) (*Client, bool) {
	client, ok := n.clients.Load(clientId)
	if !ok {
		return nil, false
	}
	return client.(*Client), ok
}

func (n *Hub) createClient(conn IConn, data *SessionData) *Client {
	if client, ok := n.FindClient(data.ClientId); ok {
		logger.Error(fmt.Sprintf("clientId【%v】已连接,关闭旧连接", data.ClientId))
		client.conn.Load().(IConn).Close()
		client.conn.Store(conn)
		return client
	} else {
		logger.Info("新增加客户端", zap.String("clientId", data.ClientId))
		client = newClient(data.ClientId, conn, &ClientOptions{
			HeartbeatTimeout: n.options.HeartbeatTimeout,
			WaitTimeout:      n.options.RetryTimeout,
			RetryInterval:    n.options.RetryInterval,
		})
		switch conn.(type) {
		case *TcpConn:
			conn.ListenToOnMessage(func(data interface{}) {
				pkt, err := packetCoder.unmarshal(data.([]byte))
				if err != nil {
					logger.Error("net通信包解析出错", zap.Error(err), zap.Any("packet", string(data.([]byte))))
					return
				}
				client.receivePacket(pkt)
			})
			conn.(*TcpConn).ListenToOnDisconnect(func(i interface{}) {
				client.ClearAllSubTopics()
			})
		case *WebsocketConn:
			conn.ListenToOnMessage(func(data interface{}) {
				pkt, err := packetCoder.unmarshal(data.([]byte))
				if err != nil {
					logger.Error("net通信包解析出错", zap.Error(err), zap.Any("packet", string(data.([]byte))))
					return
				}
				client.receivePacket(pkt)
			})
			conn.(*WebsocketConn).ListenToOnDisconnect(func(i interface{}) {
				client.ClearAllSubTopics()
			})
		}
		client.sessionId = data.SessionId
		client.handlerMgr = n.handlerMgr
		n.clients.Store(data.ClientId, client)

		if data.NodeId != nil {
			bucket := n.FindOrCreateGroup(*data.NodeId)
			bucket.AddClient(client)
		} else {
			n.AddClient(client)
		}
		client.OnDisconnect.AddEventListener(func(any interface{}) {
			n.clients.Delete(data.ClientId)
		})
		return client
	}
}

type SessionData struct {
	ClientId  string `json:"client_id"`
	NodeId    *int64 `json:"node_id"`
	SessionId string `json:"session_id,omitempty"`
}

type CheckLogin func(params LoginParams) error
