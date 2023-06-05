package main

import (
	"fmt"
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/game/pb"
	"github.com/golang/protobuf/proto"
	"log"
	"strconv"
)

type codec struct {
}

func (c *codec) Marshal(packet nethub.INetPacket) []byte {
	switch packet.TypeCode() {
	case nethub.REQUEST_PACKET:
		rawReq := packet.(*nethub.RequestPacket)
		msgId, _ := strconv.ParseInt(rawReq.Method, 10, 32)
		data, _ := proto.Marshal(&pb.Request{
			Id:     rawReq.Id,
			Method: int32(msgId),
			Params: rawReq.Params,
		})
		return append([]byte{uint8(packet.TypeCode())}, data...)
	case nethub.ACK_PACKET:
		rawAck := packet.(*nethub.AckPacket)
		data, _ := proto.Marshal(&pb.Ack{Id: rawAck.Id})
		return data
	case nethub.RESPONSE_PACKET:
		resp := packet.(*nethub.ResponsePacket)
		data, _ := proto.Marshal(&pb.Response{
			Id:     resp.Id,
			Result: resp.Result,
			Error:  resp.Error,
		})
		return append([]byte{uint8(packet.TypeCode())}, data...)
	default:
		log.Println("packet marshal err,未知包类型", packet.TypeCode())
		return nil
	}
}

// 1byte为消息类型,
func (c *codec) Unmarshal(src []byte) (nethub.INetPacket, error) {
	var typeCode = nethub.PacketTypeCode(src[:1][0])
	switch typeCode {
	case nethub.REQUEST_PACKET:
		var req pb.Request
		err := proto.Unmarshal(src[1:], &req)
		return &nethub.RequestPacket{
			Id:     req.Id,
			Method: fmt.Sprintf("%v", req.Method),
			Params: req.Params,
		}, err
	case nethub.ACK_PACKET:
		var ack pb.Ack
		err := proto.Unmarshal(src[1:], &ack)
		return &nethub.AckPacket{
			Id: ack.Id,
		}, err
	case nethub.RESPONSE_PACKET:
		var resp pb.Response
		err := proto.Unmarshal(src[1:], &resp)
		return &nethub.ResponsePacket{
			Id:     resp.Id,
			Error:  resp.Error,
			Result: resp.Result,
		}, err
	default:
		return nil, fmt.Errorf("未知消息类型[%v]", typeCode)
	}
}
