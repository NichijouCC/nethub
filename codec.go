package nethub

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

type ICodec interface {
	Marshal(packet INetPacket, crypto *Crypto) []byte
	Unmarshal(src []byte, crypto *Crypto) (INetPacket, error)
}

var defaultCodec = NewSimpleCodec()

// <type_code>@<content_data_string(json)>
type SimpleCodec struct {
	contentDecoder map[PacketTypeCode]func(packet []byte) (INetPacket, error)
}

func NewSimpleCodec() *SimpleCodec {
	var contentDecoder = map[PacketTypeCode]func(packet []byte) (INetPacket, error){
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
			var req PublishPacket
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
			var req StreamRequestRawPacket
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
			var req PingPacket
			err := json.Unmarshal(body, &req)
			return &req, err
		},
		PONG_PACKET: func(body []byte) (INetPacket, error) {
			var req PongPacket
			err := json.Unmarshal(body, &req)
			return &req, err
		},
		HEARTBEAT_PACKET: func(body []byte) (INetPacket, error) {
			var req HeartbeatPacket
			err := json.Unmarshal(body, &req)
			return &req, err
		},
	}
	return &SimpleCodec{contentDecoder: contentDecoder}
}

func (m *SimpleCodec) Unmarshal(rawPacket []byte, crypto *Crypto) (INetPacket, error) {
	packetStr := string(rawPacket)
	if strings.HasPrefix(packetStr, "@") {
		//加密了
		strArr := strings.Split(packetStr, "@")
		data := []byte(strArr[1])
		crypto.Decode(data, data)
		packetStr = string(data)
	}
	strArr := strings.Split(packetStr, "@")
	intCode, err := strconv.Atoi(strArr[0])
	if err != nil {
		return nil, errors.New("通信包头解析失败")
	}
	unmarshal, ok := m.contentDecoder[PacketTypeCode(intCode)]
	if !ok {
		return nil, errors.New("通信包头解析失败")
	}
	if len(strArr) < 2 {
		return nil, errors.New("通信包结构不对")
	}
	return unmarshal([]byte(strArr[1]))
}

func (m *SimpleCodec) Marshal(pkt INetPacket, crypto *Crypto) []byte {
	pktBytes, err := json.Marshal(pkt)
	if err != nil {
		logger.Error("json Marshal出错", zap.Error(err), zap.Any("PKT", pkt))
		return nil
	}
	pktData := []byte(fmt.Sprintf("%v@%v", pkt.TypeCode(), string(pktBytes)))
	if crypto != nil {
		crypto.Encode(pktData, pktData)
		return []byte(fmt.Sprintf("@%v", string(pktData)))
	} else {
		return pktData
	}
}
