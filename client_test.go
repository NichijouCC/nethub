package nethub

import (
	"encoding/json"
	"github.com/google/uuid"
	"log"
	"testing"
	"time"
)

func TestClient_RequestWithRetry(t *testing.T) {
	conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String()})
	conn.OnLogin.AddEventListener(func(data interface{}) {
		//远程调用
		result, err := conn.RequestWithRetry("load_data", map[string]interface{}{"a": 1, "b": 2})
		log.Println(result, err)
	})
}

func TestClient_SubscribeAttributes(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
	client.OnLogin.AddEventListener(func(data interface{}) {
		var gpsAtts = []string{"longitude", "latitude", "altitude", "yaw"}
		client.SubscribeAttributes("+/rt_message", gpsAtts, func(data *TransformedPublishPacket, from *Client) {
			log.Println(data.ClientId, string(data.Params))
		})
	})
	select {}
}

func TestClient_Subscribe(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
	client.OnLogin.AddEventListener(func(data interface{}) {
		client.Subscribe("+/rt_message", func(data *TransformedPublishPacket, from *Client) {
			log.Println(data.ClientId, string(data.Params))
		})
	})
	select {}
}

func TestClient_Publish(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
	client.OnLogin.AddEventListener(func(data interface{}) {
		//模拟实时上报消息
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			client.Publish("rt_message", `{"timestamp": 1671610334.1461706, "network_latency": 100.0, "longitude": 101.75232644, "latitude": 26.63366599, "altitude": 1116.06578656}`)
		}
	})
	select {}
}

func TestClient_StreamRequest(t *testing.T) {
	var projectId int64 = 53010217439105
	conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
	conn.OnLogin.AddEventListener(func(data interface{}) {
		//初始化流
		stream := conn.StreamRequest("load_data", nil)
		go func() {
			//stream close监听
			<-stream.CloseCh
		}()

		for j := 0; j < 100; j++ {
			//持续发送流消息
			err := stream.Request(map[string]interface{}{"pp": j})
			if err != nil {
				log.Println("error", err)
			}
		}
		//处理服务端返回的回复
		stream.OnResponse = func(pkt *StreamResponsePacket) {
			str, _ := json.Marshal(pkt)
			log.Println("client receive pkt", string(str))
		}
		//客户端主动关闭数据流
		stream.Close()
	})
	select {}
}
