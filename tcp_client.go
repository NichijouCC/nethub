package nethub

import (
	"go.uber.org/zap"
	"time"
)

func DialHubTcp(addr string, params LoginParams, opts *ClientOptions) *Client {
	var client = newClient(nil, opts)
	client.beClient.Store(true)

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
		client.conn = conn
		client.changeState(CONNECTED)
		conn.OnMessage.AddEventListener(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			logger.Error("Tcp断连.....")
			client.changeState(DISCONNECT)
			client.ClearAllSubTopics()
			time.Sleep(time.Second * 3)
			tryConn()
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
