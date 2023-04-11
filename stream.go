package nethub

type Stream struct {
	Id     string
	Client *Client
	OnRev  chan INetPacket
}

func newStream(id string, client *Client) *Stream {
	s := &Stream{
		Id:     id,
		Client: client,
		OnRev:  make(chan INetPacket, 10),
	}
	return s
}

func (s *Stream) CloseAndRev() error {
	return s.Client.SendPacketDirect(&StreamClosePacket{StreamId: s.Id})
}

func (s *Stream) Request(params interface{}) error {
	return s.Client.SendPacketDirect(&StreamRequestPacket{StreamId: s.Id, Params: params})
}

func (s *Stream) Response(result interface{}, err error) error {
	resp := &StreamResponsePacket{StreamId: s.Id, Result: result}
	if err != nil {
		resp.Error = err.Error()
	}
	return s.Client.SendPacketDirect(resp)
}
