package nethub

import "testing"

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
		req := pkt.PacketContent.(*RequestPacket)
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

	hub.RegisterStreamHandler("add", func(first *Packet, stream *Stream) {
		//初次触发,执行
		stream.OnRequest = func(pkt *StreamRequestPacket) {
			_ = pkt.Method
			_ = pkt.Params
			_ = pkt.Id
			//持续处理客户端发来的流消息
			//todo

			//发送消息到客户端,举例
			stream.RequestWithRetry("xx", nil)
			//服务端可以主动关闭数据流,举例
			stream.Close()
		}
	})
}
