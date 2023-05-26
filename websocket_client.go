package nethub

import (
	"fmt"
	"go.uber.org/zap"
	"net/url"
	"strconv"
	"time"
)

func DialHubWebsocket(addr string, params LoginParams) *Client {
	var client = newClient(nil, &ClientOptions{HeartbeatTimeout: 5, WaitTimeout: 5, RetryInterval: 3})
	values := url.Values{}
	values.Set("client_id", params.ClientId)
	if params.BucketId != nil {
		values.Set("bucket_id", strconv.FormatInt(*params.BucketId, 10))
	}
	addr = fmt.Sprintf("%v?%v", addr, values.Encode())

	var tryConn func()
	tryConn = func() {
		logger.Info("Try进行websocket连接..", zap.Any("addr", addr))
		ws, err := DialWebsocket(addr)
		if err != nil {
			logger.Info("websocket连接失败", zap.Error(err))
			time.Sleep(time.Second * 3)
			tryConn()
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
			tryConn()
		})
		ws.StartReadWrite()
		client.OnLogin.RiseEvent(nil)
	}
	go tryConn()
	return client
}
