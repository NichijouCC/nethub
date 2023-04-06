package nethub

import (
	"github.com/google/uuid"
	"sync"
)

type Stream struct {
	Id         string
	client     *Client
	OnRequest  func(pkt *StreamRequestRawPacket)
	OnResponse func(pkt *StreamResponsePacket)

	isClosed bool
	sync.Mutex
	CloseCh chan struct{}
	//异常关闭时，造成关闭的错误信息
	CloseErr error
}

func newStream(id string, client *Client) *Stream {
	s := &Stream{Id: id, client: client, CloseCh: make(chan struct{})}
	return s
}

// 正常关闭
func (s *Stream) Close() {
	s.CloseWithError(nil)
}

// 异常关闭
func (s *Stream) CloseWithError(err error) {
	s.Lock()
	defer s.Unlock()
	if s.isClosed {
		return
	}
	s.CloseErr = err
	s.isClosed = true
	close(s.CloseCh)
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
