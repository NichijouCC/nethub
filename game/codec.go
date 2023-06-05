package main

import (
	"github.com/NichijouCC/nethub"
)

type codec struct {
}

func (c *codec) Marshal(packet nethub.INetPacket) []byte {
	//TODO implement me
	panic("implement me")
}

func (c *codec) Unmarshal(src []byte) (nethub.INetPacket, error) {
	//TODO implement me
	panic("implement me")
}
