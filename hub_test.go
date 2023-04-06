package nethub

import (
	"encoding/json"
	"log"
	"sync"
	"testing"
)

func TestHub_ListenAndServe(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		RetryTimeout:     10,
		RetryInterval:    3,
	})
	hub.ListenAndServeTcp(":1235", 2)
	hub.ListenAndServeUdp(":1234", 2)
	hub.ListenAndServeWebsocket(":1555")
	select {}
}

func TestHub_RegisterRequestHandler(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		RetryTimeout:     10,
		RetryInterval:    3,
	})
	hub.ListenAndServeTcp(":1235", 2)

	hub.RegisterRequestHandler("add", func(pkt *Packet) (interface{}, error) {
		req := pkt.PacketContent.(*RequestRawPacket)
		_ = req.Params
		//todo
		return nil, nil
	})
}

func TestHub_RegisterStreamHandler(t *testing.T) {
	hub := New(&HubOptions{
		HeartbeatTimeout: 10,
		RetryTimeout:     10,
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
					stream.RequestWithRetry("xx", nil)
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
