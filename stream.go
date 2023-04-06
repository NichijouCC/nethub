package nethub

import (
	"github.com/google/uuid"
	"sync"
)

type Stream struct {
	Id     string
	client *Client
	//OnRevRequest  chan *StreamRequestRawPacket
	//OnRevResponse chan *StreamResponsePacket
	////对面不再发数据
	//OnRevClose chan *StreamClosePacket
	OnRev chan INetPacket

	//自身不再发数据
	closeSend bool
	sync.Mutex
}

func newStream(id string, client *Client) *Stream {
	s := &Stream{
		Id:     id,
		client: client,
		//OnRevRequest:  make(chan *StreamRequestRawPacket, 1),
		//OnRevResponse: make(chan *StreamResponsePacket, 1),
		//OnRevClose:    make(chan *StreamClosePacket, 1),
		OnRev: make(chan INetPacket, 10),
	}
	return s
}

func (s *Stream) CloseAndRev() error {
	return s.client.SendPacketWithRetry(&StreamClosePacket{Id: uuid.New().String(), StreamId: s.Id})
}

func (s *Stream) Request(params interface{}) error {
	return s.client.SendPacket(&StreamRequestPacket{StreamId: s.Id, Params: params})
}

func (s *Stream) RequestWithRetry(method string, params interface{}) error {
	return s.client.SendPacketWithRetry(&StreamRequestPacket{StreamId: s.Id, Id: uuid.New().String(), Method: method, Params: params})
}

func (s *Stream) Response(result interface{}, err error) error {
	resp := &StreamResponsePacket{StreamId: s.Id, Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	return s.client.SendPacket(resp)
}

func (s *Stream) ResponseWithRetry(result interface{}, err error) error {
	resp := &StreamResponsePacket{StreamId: s.Id, Id: uuid.New().String(), Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	return s.client.SendPacketWithRetry(resp)
}
