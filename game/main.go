package main

import (
	"github.com/NichijouCC/nethub"
)

func main() {
	hub := nethub.New(&nethub.HubOptions{
		HeartbeatTimeout: 15,
		RetryTimeout:     10,
		RetryInterval:    10,
	})
	hub.ListenAndServeUdp(":8899", 5, nethub.WithNeedLogin(true))
}

type LoginParams struct {
	Token  string `json:"token"`
	UserId string `json:"user_id"`
}
