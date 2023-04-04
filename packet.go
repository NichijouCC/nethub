package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

type Packet struct {
	//原始pkt
	RawData []byte
	//pkt 解析
	PacketType    PacketTypeCode
	PacketContent INetPacket
	Util          string
	Client        *Client
}

type netPacketCoder struct{}

var packetCoder = &netPacketCoder{}

type PacketTypeCode int

const (
	UNKOWN_PACKET          PacketTypeCode = 0
	REQUEST_PACKET         PacketTypeCode = 1
	ACK_PACKET             PacketTypeCode = 2
	RESPONSE_PACKET        PacketTypeCode = 3
	PUBLISH_PACKET         PacketTypeCode = 4
	SUBSCRIBE_PACKET       PacketTypeCode = 5
	UNSUBSCRIBE_PACKET     PacketTypeCode = 6
	BROADCAST_PACKET       PacketTypeCode = 7
	PING_PACKET            PacketTypeCode = 10
	PONG_PACKET            PacketTypeCode = 11
	STREAM_REQUEST_PACKET  PacketTypeCode = 20
	STREAM_RESPONSE_PACKET PacketTypeCode = 21
	STREAM_CLOSE_PACKET    PacketTypeCode = 22
)

type INetPacket interface {
	TypeCode() PacketTypeCode
	GetId() string
}

type RequestPacket struct {
	Id     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (r *RequestPacket) TypeCode() PacketTypeCode {
	return REQUEST_PACKET
}

func (r *RequestPacket) GetId() string {
	return r.Id
}

type TransformedPublishPacket struct {
	Id       string          `json:"id,omitempty"`
	Topic    string          `json:"topic"`
	Params   json.RawMessage `json:"params"`
	ClientId string          `json:"client_id,omitempty"`
}

func (r *TransformedPublishPacket) TypeCode() PacketTypeCode {
	return PUBLISH_PACKET
}

func (r *TransformedPublishPacket) GetId() string {
	return r.Id
}

type SubscribePacket struct {
	Id         string   `json:"id,omitempty"`
	Topic      string   `json:"topic"`
	Attributes []string `json:"attributes"`
}

func (r *SubscribePacket) TypeCode() PacketTypeCode {
	return SUBSCRIBE_PACKET
}

func (r *SubscribePacket) GetId() string {
	return r.Id
}

type UnSubscribePacket SubscribePacket

func (r *UnSubscribePacket) TypeCode() PacketTypeCode {
	return UNSUBSCRIBE_PACKET
}

func (r *UnSubscribePacket) GetId() string {
	return r.Id
}

type BroadcastPacket TransformedPublishPacket

func (r *BroadcastPacket) TypeCode() PacketTypeCode {
	return BROADCAST_PACKET
}

func (r *BroadcastPacket) GetId() string {
	return r.Id
}

type ResponsePacket struct {
	Id     string      `json:"id,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func (r *ResponsePacket) TypeCode() PacketTypeCode {
	return RESPONSE_PACKET
}

func (r *ResponsePacket) GetId() string {
	return r.Id
}

type AckPacket struct {
	Id string `json:"id,omitempty"`
}

func (r *AckPacket) TypeCode() PacketTypeCode {
	return ACK_PACKET
}

func (r *AckPacket) GetId() string {
	return r.Id
}

type StreamRequestPacket struct {
	StreamId string          `json:"stream_id"`
	Id       string          `json:"id,omitempty"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params"`
}

func (r *StreamRequestPacket) TypeCode() PacketTypeCode {
	return STREAM_REQUEST_PACKET
}

func (r *StreamRequestPacket) GetId() string {
	return r.Id
}

type StreamResponsePacket struct {
	StreamId string      `json:"stream_id"`
	Id       string      `json:"id,omitempty"`
	Result   interface{} `json:"result,omitempty"`
	Error    string      `json:"error,omitempty"`
}

func (r *StreamResponsePacket) TypeCode() PacketTypeCode {
	return STREAM_RESPONSE_PACKET
}

func (r *StreamResponsePacket) GetId() string {
	return r.Id
}

type StreamClosePacket struct {
	Id       string `json:"id,omitempty"`
	StreamId string `json:"stream_id"`
	Error    string `json:"error,omitempty"`
}

func (r *StreamClosePacket) TypeCode() PacketTypeCode {
	return STREAM_CLOSE_PACKET
}

func (r *StreamClosePacket) GetId() string {
	return r.Id
}

type PingPacket string

func (r *PingPacket) TypeCode() PacketTypeCode {
	return PING_PACKET
}

func (r *PingPacket) GetId() string {
	return ""
}

type PongPacket string

func (r *PongPacket) TypeCode() PacketTypeCode {
	return PONG_PACKET
}

func (r *PongPacket) GetId() string {
	return ""
}

var netPacketUnmarshalDic = map[PacketTypeCode]func(packet []byte) (INetPacket, error){
	REQUEST_PACKET: func(body []byte) (INetPacket, error) {
		var req RequestPacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	ACK_PACKET: func(body []byte) (INetPacket, error) {
		var req AckPacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	RESPONSE_PACKET: func(body []byte) (INetPacket, error) {
		var req ResponsePacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	PUBLISH_PACKET: func(body []byte) (INetPacket, error) {
		var req TransformedPublishPacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	SUBSCRIBE_PACKET: func(body []byte) (INetPacket, error) {
		var req SubscribePacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	UNSUBSCRIBE_PACKET: func(body []byte) (INetPacket, error) {
		var req UnSubscribePacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	BROADCAST_PACKET: func(body []byte) (INetPacket, error) {
		var req BroadcastPacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	STREAM_REQUEST_PACKET: func(body []byte) (INetPacket, error) {
		var req StreamRequestPacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	STREAM_RESPONSE_PACKET: func(body []byte) (INetPacket, error) {
		var req StreamResponsePacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	STREAM_CLOSE_PACKET: func(body []byte) (INetPacket, error) {
		var req StreamClosePacket
		err := json.Unmarshal(body, &req)
		return &req, err
	},
	PING_PACKET: func(body []byte) (INetPacket, error) {
		var req = PingPacket(body)
		return &req, nil
	},
	PONG_PACKET: func(body []byte) (INetPacket, error) {
		var req = PingPacket(body)
		return &req, nil
	},
}

func (m *netPacketCoder) unmarshal(rawPacket []byte) (*Packet, error) {
	pkt := &Packet{}
	pkt.RawData = rawPacket
	packetStr := string(rawPacket)
	strArr := strings.Split(packetStr, "@")
	intCode, err := strconv.Atoi(strArr[0])
	if err != nil {
		return nil, errors.New("通信包头解析失败")
	}
	pkt.PacketType = PacketTypeCode(intCode)
	unmarshal, ok := netPacketUnmarshalDic[pkt.PacketType]
	if !ok {
		return nil, errors.New("通信包头解析失败")
	}
	if len(strArr) < 2 {
		return nil, errors.New("通信包结构不对")
	}
	pkt.PacketContent, err = unmarshal([]byte(strArr[1]))
	if err != nil {
		return nil, errors.New("通信包体解析失败")
	}
	if len(strArr) >= 3 {
		pkt.Util = strArr[2]
	}
	return pkt, nil
}

func (m *netPacketCoder) marshal(pkt INetPacket) []byte {
	data, err := json.Marshal(pkt)
	if err != nil {
		logger.Error("json Marshal出错", zap.Error(err), zap.Any("PKT", pkt))
		return nil
	}
	dataStr := fmt.Sprintf("%v@%v", pkt.TypeCode(), string(data))
	return []byte(dataStr)
}

func (m *netPacketCoder) marshalWithSessionId(pkt INetPacket, sessionId string) []byte {
	data, _ := json.Marshal(pkt)
	dataStr := fmt.Sprintf("%v@%v@%v", pkt.TypeCode(), string(data), sessionId)
	return []byte(dataStr)
}

func Marshal(pkt INetPacket) []byte {
	return packetCoder.marshal(pkt)
}

func Unmarshal(rawPacket []byte) (*Packet, error) {
	return packetCoder.unmarshal(rawPacket)
}

type PublishPacket struct {
	Id     string      `json:"id,omitempty"`
	Topic  string      `json:"topic"`
	Params interface{} `json:"params"`
}

func (r *PublishPacket) TypeCode() PacketTypeCode {
	return PUBLISH_PACKET
}

func (r *PublishPacket) GetId() string {
	return r.Id
}
