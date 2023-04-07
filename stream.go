package nethub

import (
	"github.com/google/uuid"
)

type Stream struct {
	Id     string
	client *Client
	OnRev  chan INetPacket
}

func newStream(id string, client *Client) *Stream {
	s := &Stream{
		Id:     id,
		client: client,
		OnRev:  make(chan INetPacket, 10),
	}
	return s
}

func (s *Stream) CloseAndRev() error {
	return s.client.SendPacketWithRetry(&StreamClosePacket{Id: uuid.New().String(), StreamId: s.Id})
}

func (s *Stream) Request(params interface{}) error {
	return s.client.SendPacketDirect(&StreamRequestPacket{StreamId: s.Id, Params: params})
}

func (s *Stream) RequestWithRetry(method string, params interface{}) error {
	return s.client.SendPacketWithRetry(&StreamRequestPacket{StreamId: s.Id, Id: uuid.New().String(), Method: method, Params: params})
}

func (s *Stream) Response(result interface{}, err error) error {
	resp := &StreamResponsePacket{StreamId: s.Id, Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	return s.client.SendPacketDirect(resp)
}

func (s *Stream) ResponseWithRetry(result interface{}, err error) error {
	resp := &StreamResponsePacket{StreamId: s.Id, Id: uuid.New().String(), Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	return s.client.SendPacketWithRetry(resp)
}
