package nethub

import (
	"encoding/json"
)

type Packet struct {
	//原始pkt
	RawData []byte
	//pkt类型
	PacketType PacketTypeCode
	//pkt内容包
	PacketContent INetPacket
	Util          string
	Client        *Client
}

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
	Id     string      `json:"id,omitempty"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

func (r *RequestPacket) TypeCode() PacketTypeCode {
	return REQUEST_PACKET
}

func (r *RequestPacket) GetId() string {
	return r.Id
}

type RequestRawPacket struct {
	Id     string          `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (r *RequestRawPacket) TypeCode() PacketTypeCode {
	return REQUEST_PACKET
}

func (r *RequestRawPacket) GetId() string {
	return r.Id
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

type PublishRawPacket struct {
	Id       string          `json:"id,omitempty"`
	Topic    string          `json:"topic"`
	Params   json.RawMessage `json:"params"`
	ClientId string          `json:"client_id,omitempty"`
}

func (r *PublishRawPacket) TypeCode() PacketTypeCode {
	return PUBLISH_PACKET
}

func (r *PublishRawPacket) GetId() string {
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

type BroadcastPacket PublishRawPacket

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
	StreamId string      `json:"stream_id"`
	Id       string      `json:"id,omitempty"`
	Method   string      `json:"method,omitempty"`
	Params   interface{} `json:"params"`
}

func (r *StreamRequestPacket) TypeCode() PacketTypeCode {
	return STREAM_REQUEST_PACKET
}

func (r *StreamRequestPacket) GetId() string {
	return r.Id
}

type StreamRequestRawPacket struct {
	StreamId string          `json:"stream_id"`
	Id       string          `json:"id,omitempty"`
	Method   string          `json:"method,omitempty"`
	Params   json.RawMessage `json:"params"`
}

func (r *StreamRequestRawPacket) TypeCode() PacketTypeCode {
	return STREAM_REQUEST_PACKET
}

func (r *StreamRequestRawPacket) GetId() string {
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
