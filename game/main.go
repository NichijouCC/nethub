package main

import (
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/aoi"
	"github.com/NichijouCC/nethub/game/pb"
	"github.com/golang/protobuf/proto"
)

func main() {
	InitConfig()
	InitMysql(Config.Mysql)
	aoi.InitMapWorld(map[int32]aoi.MapData{
		1: {212, 392, 267, 423, 30, 30, 340, 88, 280, 0},
		2: {-116, 0, -54, 83, 30, 30, -40, 0, -30, 0},
	})
	aoi.NotifyMove = func(player *aoi.Player, pos aoi.PosMessage) {
		if value, ok := player.GetProperty("from"); ok {
			from := value.(*nethub.Client)
			params := pb.UserMoveReq{Position: pos.Position, Speed: pos.Speed, Timestamp: pos.Timestamp}
			data, _ := proto.Marshal(&params)
			from.SendPacket(&nethub.RequestPacket{
				Method: "23",
				Params: data,
			})
		}
	}
	aoi.NotifyLeave = func(player *aoi.Player, leave []int64) {
		if value, ok := player.GetProperty("from"); ok {
			from := value.(*nethub.Client)
			params := pb.UserLeaveReq{UserIds: leave}
			data, _ := proto.Marshal(&params)
			from.SendPacket(&nethub.RequestPacket{
				Method: "24",
				Params: data,
			})
		}
	}
	aoi.NotifyAroundLive = func(player *aoi.Player, live []int64) {
		if value, ok := player.GetProperty("from"); ok {
			from := value.(*nethub.Client)
			params := pb.AroundLivesReq{UserIds: live}
			data, _ := proto.Marshal(&params)
			from.SendPacket(&nethub.RequestPacket{
				Method: "25",
				Params: data,
			})
		}
	}
	InitPlayerMgr()
	InitChatMgr()

	hub := nethub.New(&nethub.HubOptions{
		HeartbeatInterval: 0,
		HeartbeatTimeout:  0,
		WaitTimeout:       10,
		RetryInterval:     10,
	})

	tcpService := InitTcpService()
	udpService := InitUdpService()

	hub.ListenAndServeTcp(":8998", 1, nethub.WithHandlerMgr(tcpService), nethub.WithCodec(&codec{}))
	hub.ListenAndServeUdp(":8999", 1, nethub.WithHandlerMgr(udpService), nethub.WithCodec(&codec{}))

	select {}
}
