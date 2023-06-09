package nethub

import (
	"encoding/json"
	"github.com/google/uuid"
	"log"
	"testing"
	"time"
)

func TestClient_Request(t *testing.T) {
	conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String()}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
	})
	conn.OnLogin.AddEventListener(func(data interface{}) {
		//远程调用
		param, _ := json.Marshal(map[string]interface{}{"a": 1, "b": 2})
		result, err := conn.Request("load_data", param)
		log.Println(result, err)
	})
}

func TestClient_RequestWithRetry(t *testing.T) {
	conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String()}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
	})
	conn.OnLogin.AddEventListener(func(data interface{}) {
		//远程调用
		param, _ := json.Marshal(map[string]interface{}{"a": 1, "b": 2})
		result, err := conn.RequestWithRetry("load_data", param)
		log.Println(result, err)
	})
}

func TestClient_SubscribeAttributes(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
		NeedLogin:        true,
	})
	client.OnLogin.AddEventListener(func(data interface{}) {
		var gpsAtts = []string{"longitude", "latitude", "altitude", "yaw"}
		client.SubscribeAttributes("+/rt_message", gpsAtts, func(data *PublishPacket, from *Client) {
			log.Println(data.ClientId, string(data.Params))
		})
	})
	select {}
}

func TestClient_Subscribe(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
	})
	client.OnLogin.AddEventListener(func(data interface{}) {
		client.Subscribe("+/rt_message", func(data *PublishPacket, from *Client) {
			log.Println(data.ClientId, string(data.Params))
		})
	})
	select {}
}

func TestClient_websocket_Subscribe(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubWebsocket("ws://127.0.0.1:1555", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
	client.OnLogin.AddEventListener(func(data interface{}) {
		go client.Subscribe("+/rt_message", func(data *PublishPacket, from *Client) {
			log.Println(data.ClientId, string(data.Params))
		})
	})
	select {}
}

func TestClient_Publish(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
		NeedLogin:        true,
	})
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		if client.state.Load().(clientState) == LOGINED {
			client.Publish("rt_message", []byte(`{"timestamp": 1671610334.1461706, "network_latency": 100.0, "longitude": 101.75232644, "latitude": 26.63366599, "altitude": 1116.06578656}`))
		}
	}
	select {}
}

func TestClient_StreamRequest(t *testing.T) {
	var projectId int64 = 53010217439105
	conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
	})
	conn.OnLogin.AddEventListener(func(data interface{}) {
		//初始化流
		conn.StreamRequest("add", nil, func(stream *Stream) error {
			go func() {
				for j := 0; j < 5; j++ {
					//持续发送流消息
					err := stream.Request(map[string]interface{}{"pp": j})
					if err != nil {
						log.Println("error", err)
					}
				}
				stream.CloseAndRev()
				log.Println("client stream close send and rev")
			}()
		For:
			for pkt := range stream.OnRev {
				switch pkt.(type) {
				case *StreamRequestRawPacket:
					//处理服务端返回的回复
					str, _ := json.Marshal(pkt.(*StreamRequestRawPacket))
					log.Println("client receive req pkt", string(str))
				case *StreamResponsePacket:
					//处理服务端返回的回复
					str, _ := json.Marshal(pkt.(*StreamResponsePacket))
					log.Println("client receive resp pkt", string(str))
				case *StreamClosePacket:
					str, _ := json.Marshal(pkt.(*StreamClosePacket))
					log.Println("client receive close pkt", string(str))
					break For
				}
			}

			log.Println("stream结束")
			return nil
		})
	})
	select {}
}

func TestClientPing(t *testing.T) {
	var projectId int64 = 53010217439105
	client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId}, &ClientOptions{
		HeartbeatTimeout: 5,
		WaitTimeout:      5,
		RetryInterval:    3,
		Crypto:           NewCrypto(),
	})
	client.OnLogin.AddEventListener(func(data interface{}) {
		client.OnPongHandler = func(pkt *PongPacket) {
			log.Println(*pkt)
		}
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			var pkt = PingPacket(time.Now().String())
			client.SendPacket(&pkt)
		}
	})
	select {}
}
