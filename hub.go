package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
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
	cacheSession sync.Map
	//sessionId->*NetClient
	usingSession sync.Map
	//clientId->*NetClient
	clients sync.Map
	//bucketId->*NetBucket
	buckets sync.Map
	*Bucket
	*handlerMgr
	//用于login认证
	checkLogin func(params *LoginParams) bool
	options    *HubOptions
}

func New(options *HubOptions) *Hub {
	r := &Hub{
		Bucket:     newBucket(-1, nil),
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
	udp.OnReceiveMessage = func(rawData []byte, conn *udpConn) {
		pkt, err := packetCoder.unmarshal(rawData)
		if err != nil {
			logger.Error("net通信包解析出错", zap.Any("packet", string(rawData)))
			return
		}
		if pkt.Util != "" {
			if value, loaded := n.usingSession.Load(pkt.Util); loaded {
				client := value.(*Client)
				client.conn.Store(conn)
				client.receivePacket(pkt)
			} else {
				if value, loaded = n.cacheSession.LoadAndDelete(pkt.Util); loaded {
					data := value.(*sessionData)
					client := n.createClient(conn, data)
					client.conn.Store(conn)
					client.receivePacket(pkt)
				}
			}
		} else {
			if pkt.PacketType == REQUEST_PACKET && pkt.PacketContent.(*RequestRawPacket).Method == "login" {
				req := pkt.PacketContent.(*RequestRawPacket)
				var params LoginParams
				err = json.Unmarshal(req.Params, &params)
				if err != nil {
					conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Error: "认证失败"}))
					return
				}
				session, err := n.login(params)
				if err != nil {
					conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Error: "认证失败"}))
					return
				}
				conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Result: session.SessionId}))
				//创建client
				client := n.createClient(conn, session)
				go client.LoopCheckAlive()
			}
			return
		}
	}
	udp.ListenAndServe(listenerCount)
}

func (n *Hub) ListenAndServeTcp(addr string, listenerCount int) {
	//tcp Server
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
				if n.checkLogin != nil && !n.checkLogin(&params) {
					conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Error: "认证失败"}))
					return "", errors.New("认证失败")
				}
				conn.SendMessage(packetCoder.marshal(&ResponsePacket{Id: req.Id, Result: "认证成功"}))
				//创建client
				n.createClient(conn, &sessionData{
					ClientId:  params.ClientId,
					NodeId:    params.BucketId,
					SessionId: uuid.New().String(),
				})
				return true, nil
			} else {
				return nil, errors.New("认证失败")
			}
		},
	}))
	go tcp.ListenAndServe(listenerCount)
}

func (n *Hub) ListenAndServeWebsocket(addr string) {
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
			if n.checkLogin != nil && !n.checkLogin(&params) {
				return nil, errors.New("认证失败")
			}
			return &sessionData{
				ClientId:  params.ClientId,
				NodeId:    params.BucketId,
				SessionId: uuid.New().String(),
			}, nil
		},
	}))
	ws.OnClientConnect = func(conn *WebsocketConn) {
		n.createClient(conn, conn.auth.(*sessionData))
	}
	go ws.ListenAndServe(n.httpLogin)
}

func (n *Hub) findOrCreateBucket(id int64) *Bucket {
	if value, loaded := n.buckets.Load(id); loaded {
		return value.(*Bucket)
	}
	value, _ := n.buckets.LoadOrStore(id, newBucket(id, n.Bucket))
	return value.(*Bucket)
}

type HttpResp struct {
	Code    int         `json:"code,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

func (n *Hub) handleRequest(pkt *Packet) (interface{}, error) {
	handler, ok := n.findRequestHandler(pkt.PacketContent.(*RequestRawPacket).Method)
	if !ok {
		return nil, errors.New("无法找到对应方法")
	}
	var executeErr error
	var result interface{}
	func() {
		defer func() {
			if err := recover(); err != nil {
				executeErr = err.(error)
				fmt.Println("rpc execute exception:", err)
			}
		}()
		result, executeErr = handler(pkt)
	}()
	return result, executeErr
}

func (n *Hub) handleStream(first *Packet, stream *Stream) {
	handler, ok := n.findStreamHandler(first.PacketContent.(*StreamRequestRawPacket).Method)
	if !ok {
		stream.CloseWithError(errors.New("无法找到对应方法"))
		return
	}
	func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("stream execute exception:", err)
				stream.CloseWithError(errors.New("服务内部执行异常"))
			}
		}()
		handler(first, stream)
	}()
}

// udp可以通过https安全的获得sessionId
func (n *Hub) httpLogin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		resp := &HttpResp{Code: 401, Message: "Body Read出错"}
		data, _ := json.Marshal(resp)
		w.Write(data)
		return
	}
	var params LoginParams
	err = json.Unmarshal(body, &params)
	if err != nil {
		resp := &HttpResp{Code: 401, Message: "Body Unmarshal出错"}
		data, _ := json.Marshal(resp)
		w.Write(data)
		return
	}
	session, err := n.login(params)
	if err != nil {
		resp := &HttpResp{Code: 401, Message: "认证失败"}
		data, _ := json.Marshal(resp)
		w.Write(data)
		return
	}
	resp := &HttpResp{Code: 200, Message: "登录成功", Data: session.SessionId}
	data, _ := json.Marshal(resp)
	w.Write(data)
}

// 返回sessionId
func (n *Hub) login(params LoginParams) (*sessionData, error) {
	if n.checkLogin != nil && !n.checkLogin(&params) {
		return nil, errors.New("认证失败")
	}
	auth := &sessionData{NodeId: params.BucketId, ClientId: params.ClientId, SessionId: uuid.New().String()}
	n.cacheSession.Store(auth.SessionId, auth)
	go func() {
		time.Sleep(time.Minute * 2)
		n.cacheSession.Delete(auth.SessionId)
	}()
	return auth, nil
}

func (n *Hub) findCacheSession(sessionId string) (*sessionData, bool) {
	value, ok := n.cacheSession.Load(sessionId)
	if !ok {
		return nil, false
	}
	data := value.(*sessionData)
	return data, true
}

func (n *Hub) findClient(clientId string) (*Client, bool) {
	client, ok := n.clients.Load(clientId)
	if !ok {
		return nil, false
	}
	return client.(*Client), ok
}

func (n *Hub) createClient(conn IConn, data *sessionData) *Client {
	if client, ok := n.findClient(data.ClientId); ok {
		logger.Error(fmt.Sprintf("clientId【%v】已连接,关闭旧连接", data.ClientId))
		client.Close()
	}

	logger.Info("新增加客户端", zap.String("clientId", data.ClientId))
	client := newClient(data.ClientId, conn, &ClientOptions{
		HeartbeatTimeout: n.options.HeartbeatTimeout,
		RetryTimeout:     n.options.RetryTimeout,
		RetryInterval:    n.options.RetryInterval,
	})
	switch conn.(type) {
	case *TcpConn, *WebsocketConn:
		conn.ListenToOnDisconnect(func(i interface{}) {
			client.Close()
		})
		conn.ListenToOnMessage(func(data interface{}) {
			pkt, err := packetCoder.unmarshal(data.([]byte))
			if err != nil {
				logger.Error("net通信包解析出错", zap.Error(err), zap.Any("packet", string(data.([]byte))))
				return
			}
			client.receivePacket(pkt)
		})
	}
	client.sessionId = data.SessionId
	client.handlerMgr = n.handlerMgr
	n.clients.Store(data.ClientId, client)
	n.usingSession.Store(data.SessionId, client)

	if data.NodeId != nil {
		bucket := n.findOrCreateBucket(*data.NodeId)
		bucket.AddClient(client)
	} else {
		n.AddClient(client)
	}
	client.OnDisconnect.AddEventListener(func(any interface{}) {
		n.clients.Delete(data.ClientId)
		n.usingSession.Delete(data.SessionId)
	})
	return client
}

type sessionData struct {
	ClientId  string `json:"client_id"`
	NodeId    *int64 `json:"node_id"`
	SessionId string `json:"session_id,omitempty"`
}
