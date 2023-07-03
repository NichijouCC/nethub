package main

import (
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/game/pb"
	"github.com/golang/protobuf/proto"
	"log"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	var client = nethub.NewClient(nil, &nethub.ClientOptions{
		HeartbeatTimeout: 0,
		WaitTimeout:      10,
		RetryInterval:    3,
		PacketCodec:      &codec{},
		HandlerMgr:       &nethub.HandlerMgr{},
	})
	client.BeClient.Store(true)
	var tryConn func()
	tryConn = func() {
		log.Println("Try进行udp连接..")
		conn, err := nethub.DialUdp("127.0.0.1:8999")
		if err != nil {
			log.Println("udp连接失败", err)
			time.Sleep(time.Second * 3)
			go tryConn()
			return
		}
		client.Conn = conn
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			log.Println("udp断开连接")
			client.ClearAllSubTopics()
			time.Sleep(3 * time.Second)
			go tryConn()
		})
		conn.OnMessage.AddEventListener(func(data interface{}) {
			client.ReceiveMessage(data.([]byte))
		})
		conn.StartReadWrite(5)
		for {
			initParams, _ := proto.Marshal(&pb.InitUser_Req{UserId: 1, Token: "123"})
			init, err := client.Request("21", initParams)
			if err != nil {
				log.Println("登录失败", err.Error())
				time.Sleep(3 * time.Second)
			} else {
				var initData pb.InitUser_Resp
				err := proto.Unmarshal(init.([]byte), &initData)
				log.Println("登录成功", initData.MapId, initData.Position, initData.Yaw, err)

				//ticker := time.NewTicker(time.Second)
				//for range ticker.C {
				//	pos := &pb.UserMoveReq{
				//		Position:  initData.Position,
				//		Yaw:       0,
				//		Speed:     0,
				//		Timestamp: 0,
				//	}
				//	client.SendPacket(&nethub.RequestPacket{
				//		Method:   "22",
				//		Params:   nil,
				//		ClientId: "",
				//	})
				//}
				break
			}
		}
	}
	go tryConn()
	select {}
}

func TestClient2(t *testing.T) {
	var client = nethub.NewClient(nil, &nethub.ClientOptions{
		HeartbeatTimeout: 0,
		WaitTimeout:      10,
		RetryInterval:    3,
		PacketCodec:      &codec{},
		HandlerMgr:       &nethub.HandlerMgr{},
	})
	client.BeClient.Store(true)
	var tryConn func()
	tryConn = func() {
		log.Println("Try进行udp连接..")
		conn, err := nethub.DialUdp("127.0.0.1:8999")
		if err != nil {
			log.Println("udp连接失败", err)
			time.Sleep(time.Second * 3)
			go tryConn()
			return
		}
		client.Conn = conn
		conn.OnDisconnect.AddEventListener(func(data interface{}) {
			log.Println("udp断开连接")
			client.ClearAllSubTopics()
			time.Sleep(3 * time.Second)
			go tryConn()
		})
		conn.OnMessage.AddEventListener(func(data interface{}) {
			client.ReceiveMessage(data.([]byte))
		})
		conn.StartReadWrite(5)
		for {
			initParams, _ := proto.Marshal(&pb.InitUser_Req{UserId: 2, Token: "123"})
			init, err := client.Request("21", initParams)
			if err != nil {
				log.Println("登录失败", err.Error())
				time.Sleep(3 * time.Second)
			} else {
				var initData pb.InitUser_Resp
				err := proto.Unmarshal(init.([]byte), &initData)
				log.Println("登录成功", initData.MapId, initData.Position, initData.Yaw, err)

				//ticker := time.NewTicker(time.Second)
				//for range ticker.C {
				//	pos := &pb.UserMoveReq{
				//		Position:  initData.Position,
				//		Yaw:       0,
				//		Speed:     0,
				//		Timestamp: 0,
				//	}
				//	client.SendPacket(&nethub.RequestPacket{
				//		Method:   "22",
				//		Params:   nil,
				//		ClientId: "",
				//	})
				//}
				break
			}
		}
	}
	go tryConn()
	select {}
}
