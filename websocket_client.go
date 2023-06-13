package nethub

import (
	"go.uber.org/zap"
	"time"
)

func DialHubWebsocket(addr string, params LoginParams, opts *ClientOptions) *Client {
	var client = newClient(nil, opts)
	client.beClient.Store(true)

	var tryConn func()
	tryConn = func() {
		logger.Info("Try进行websocket连接..", zap.Any("addr", addr))
		ws, err := DialWebsocket(addr)
		if err != nil {
			logger.Info("websocket连接失败", zap.Error(err))
			time.Sleep(time.Second * 3)
			go tryConn()
			return
		}
		client.conn = ws
		ws.OnMessage.AddEventListener(func(data interface{}) {
			client.receiveMessage(data.([]byte))
		})
		ws.OnDisconnect.AddEventListener(func(data interface{}) {
			logger.Error("websocket断连.....")
			client.ClearAllSubTopics()
			time.Sleep(time.Second * 3)
			go tryConn()
		})
		ws.StartReadWrite()
		for {
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
