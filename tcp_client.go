package nethub

import (
	"go.uber.org/zap"
	"time"
)

func DialHubTcp(addr string, params LoginParams, opts *ClientOptions) *Client {
	var client = NewClient(nil, opts)
	client.BeClient.Store(true)

	var tryConn func()
	tryConn = func() {
		logger.Info("Try进行Tcp连接..", zap.Any("addr", addr))
		conn, err := DialTcp(addr)
		if err != nil {
			logger.Info("Tcp连接失败", zap.Error(err))
			time.Sleep(time.Second * 3)
			go tryConn()
			return
		}
		client.Conn = conn
		conn.OnMessage.AddEventListener(func(data interface{}) {
			client.ReceiveMessage(data.([]byte))
		})
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			logger.Error("Tcp断连.....")
			client.ClearAllSubTopics()
			time.Sleep(time.Second * 3)
			go tryConn()
		})
		conn.StartReadWrite()
		for {
			if conn.IsClosed() {
				return
			}
			err = client.Login(&params)
			if err != nil {
				logger.Error("登录失败", zap.Error(err), zap.Any("login params", params))
				time.Sleep(time.Second * 3)
			} else {
				break
			}
		}

	}
	go tryConn()
	return client
}
