package main

import (
	"errors"
	"fmt"
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/aoi"
	"github.com/NichijouCC/nethub/game/pb"
	"github.com/golang/protobuf/proto"
	"time"
)

//1.登录 1

//2.地图服务(udp)
//2.1初始化玩家 21 （用户to服务器）
//2.2通知位置到服务器 22 （用户to服务器）
//2.3接收其他玩家移动通知 23 （服务器to用户）
//2.4接收其他玩家消失通知 24 （服务器to用户）
//2.5接收定时同步周边存在玩家 25 (服务器to用户)
//2.6请求跳转地图（mapId,xyz） 26 (用户to服务器)

//3.聊天服务(tcp)
//3.1 发送世界聊天 31 （用户to服务器）
//3.1 接收世界聊天通知 31（服务器to用户）
//3.2 发送组队聊天 32 （用户to服务器）
//3.2 接收组队聊天 32 （服务器to用户）
//3.3 发送私聊 33 （用户to服务器）
//3.3 接收私聊通知 33（服务器to用户）
//3.4 本地聊天（周边聊天）34 （用户to服务器）
//3.4 接收本地聊天通知 34 （服务器to用户）

//4. 副本服务
//4.1进入副本（离开地图）41
//4.2玩家积分结算 42

func InitTcpService() *nethub.HandlerMgr {
	var service = &nethub.HandlerMgr{}
	service.RegisterRequestHandler("1", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.Login_Req
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		user, err := chatMgr.InitUser(params.UserId, from)
		if err != nil {
			return nil, err
		}
		from.SetProperty("player", user)
		return []byte("成功"), nil
	})

	service.RegisterRequestHandler("31", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.WorldChatReq
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		value.(*ChatUser).ChatInWorld(params)
		return nil, nil
	})

	service.RegisterRequestHandler("32", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.GroupChatReq
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		value.(*ChatUser).ChatInGroup(params)
		return nil, nil
	})

	service.RegisterRequestHandler("33", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.PrivateChatReq
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		value.(*ChatUser).ChatToUser(params)
		return nil, nil
	})

	service.RegisterRequestHandler("34", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.LocalChatReq
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		value.(*ChatUser).ChatToLocal(params)
		return nil, nil
	})

	return service
}

func InitUdpService() *nethub.HandlerMgr {
	var service = &nethub.HandlerMgr{}

	//初始化玩家
	service.RegisterRequestHandler("21", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.InitUser_Req
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		//todo,校验
		from.ClientId = fmt.Sprintf("%v", params.UserId)
		player, err := playerMgr.InitUser(params.UserId, from)
		if err != nil {
			return nil, err
		}
		from.SetProperty("player", player)
		var resp pb.InitUser_Resp
		resp.Position = player.Ins.GetPosition()
		resp.Yaw = player.Ins.GetYaw()
		resp.MapId = player.Ins.CurrentMap.Id
		from.BeLogin.Store(true)
		data, err := proto.Marshal(&resp)
		if err != nil {
			return nil, errors.New("RESP参数编码失败")
		}
		return data, nil
	})

	//玩家移动
	service.RegisterRequestHandler("22", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.UserMoveReq
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		value.(*MapPlayer).Ins.UpdatePos(aoi.PosMessage{
			Position:  params.Position,
			Yaw:       params.Yaw,
			Speed:     params.Speed,
			Timestamp: float32(time.Now().UnixMilli()) / 1000,
		})
		return nil, nil
	})
	//跳转地图
	service.RegisterRequestHandler("26", func(req *nethub.RequestPacket, from *nethub.Client) ([]byte, error) {
		var params pb.JumpMap_Req
		err := proto.Unmarshal(req.Params, &params)
		if err != nil {
			return nil, errors.New("REQ参数解析失败")
		}
		value, ok := from.GetProperty("player")
		if !ok {
			return nil, errors.New("玩家还未初始化")
		}
		ins := value.(*MapPlayer).Ins
		ins.EnterMapByInitPos(params.MapId)
		var resp = pb.JumpMap_Resp{
			Position: ins.GetPosition(),
			Yaw:      ins.GetYaw(),
		}
		data, _ := proto.Marshal(&resp)
		return data, nil
	})

	return service
}
