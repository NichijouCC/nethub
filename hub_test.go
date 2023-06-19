package nethub

import (
	"encoding/json"
	"log"
	"sync"
	"testing"
)

func TestHub_ListenAndServe(t *testing.T) {
	hub := New(&HubOptions{
		WaitTimeout:   10,
		RetryInterval: 3,
	})
	hub.ListenAndServeUdp(":1234", 2)
	hub.ListenAndServeTcp(":1235", 2)
	hub.ListenAndServeWebsocket(":1555")
	select {}
}

func TestHub_RegisterRequestHandler(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		WaitTimeout:      10,
		RetryInterval:    3,
	})
	hub.ListenAndServeTcp(":1235", 2)
	hub.RegisterRequestHandler("add", func(req *RequestPacket, from *Client) ([]byte, error) {
		data, _ := json.Marshal("2")
		return data, nil
	})
	select {}
}

func TestHub_RegisterStreamHandler(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		WaitTimeout:      10,
		RetryInterval:    3,
	})
	hub.ListenAndServeTcp(":1235", 2)

	go hub.RegisterStreamHandler("add", func(first *Packet, stream *Stream) error {
		var wait sync.WaitGroup
	For:
		for pkt := range stream.OnRev {
			switch pkt.(type) {
			case *StreamRequestRawPacket:
				//处理服务端返回的回复
				str, _ := json.Marshal(pkt.(*StreamRequestRawPacket))
				log.Println("server receive req pkt", string(str))
				//发送消息到客户端,举例
				wait.Add(1)
				go func() {
					stream.Request(map[string]interface{}{"xx": 1})
					wait.Done()
				}()
			case *StreamResponsePacket:
				//处理服务端返回的回复
				str, _ := json.Marshal(pkt.(*StreamResponsePacket))
				log.Println("server receive resp pkt", string(str))
			case *StreamClosePacket:
				str, _ := json.Marshal(pkt.(*StreamClosePacket))
				log.Println("server receive close pkt", string(str))
				break For
			}
		}
		wait.Wait()
		log.Println("stream结束")
		return nil
	})

	select {}
}

func TestHub_subscribe(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		WaitTimeout:      10,
		RetryInterval:    3,
	})
	hub.ListenAndServeTcp(":1235", 2)

	hub.SubTopic(NewTopicListener("+/+/rt_message", func(pkt *PublishPacket, from *Client) {

	}))
}

func TestJson(t *testing.T) {
	type testReq struct {
		Id     string
		Method string
		Params interface{}
	}
	req := testReq{
		Id:     "123231",
		Method: "tesst",
		Params: map[string]interface{}{"a": 1},
	}
	reqBytes, _ := json.Marshal(&req)

	type TestUnMarshal1 struct {
		Id     string
		Method string
		Params interface{}
	}
	var test1 TestUnMarshal1
	json.Unmarshal(reqBytes, &test1)
	log.Println(test1)

	type TestUnMarshal2 struct {
		Id     string
		Method string
		Params []byte
	}
	var test2 TestUnMarshal2
	json.Unmarshal(reqBytes, &test2)

	var params2 = make(map[string]interface{})
	json.Unmarshal(test2.Params, &params2)

	log.Println(test2, params2)

	req2 := testReq{
		Id:     "123231",
		Method: "tesst",
		Params: "1231",
	}
	reqBytes2, _ := json.Marshal(&req2)
	var test3 TestUnMarshal1
	json.Unmarshal(reqBytes2, &test3)
	log.Println(test3)

	var test4 TestUnMarshal2
	json.Unmarshal(reqBytes2, &test4)

	var params4 string
	json.Unmarshal(test4.Params, &params4)

	log.Println(test4, params4)
}
