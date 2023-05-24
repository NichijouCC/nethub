package nethub

import (
	"go.uber.org/zap"
	"time"
)

func DialHubTcp(addr string, params LoginParams) *Client {
	var client = newClient(params.ClientId, nil, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
	})
	var tryConn func()
	tryConn = func() {
		logger.Info("Try进行Tcp连接..", zap.Any("addr", addr))
		conn, err := DialTcp(addr)
		if err != nil {
			logger.Info("Tcp连接失败", zap.Error(err))
			time.Sleep(time.Second * 3)
			tryConn()
			return
		}
		conn.OnMessage.AddEventListener(func(data interface{}) {
			pkt, err := defaultCodec.Unmarshal(data.([]byte))
			if err != nil {
				logger.Error("net通信包解析出错", zap.Any("packet", string(data.([]byte))))
				return
			}
			client.receivePacket(pkt)
		})
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			logger.Error("Tcp断连.....")
			client.ClearAllSubTopics()
			time.Sleep(time.Second * 3)
			tryConn()
		})
		conn.StartReadWrite()
		client.conn.Store(conn)
		for {
			if conn.IsClosed() {
				return
			}
			_, err = client.Login(&params)
			if err != nil {
				logger.Error("登录失败", zap.Error(err), zap.Any("login params", params))
				time.Sleep(time.Second * 3)
			} else {
				break
			}
		}
		client.OnLogin.RiseEvent(nil)
	}
	go tryConn()
	return client
}
