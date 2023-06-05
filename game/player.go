package main

import (
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/aoi"
	"sync"
	"time"
)

var playerMgr *PlayerManager

func InitPlayerMgr() {
	playerMgr = &PlayerManager{}
	ticker := time.NewTicker(time.Second * 15)
	go func() {
		for range ticker.C {
			playerMgr.players.Range(func(key, value any) bool {
				value.(*MapPlayer).Ins.TickNotifyAroundLive()
				return true
			})
		}
	}()
}

type PlayerManager struct {
	players sync.Map
}

func (p *PlayerManager) FindPlayer(userId int64) (*MapPlayer, bool) {
	value, ok := p.players.Load(userId)
	if ok {
		return value.(*MapPlayer), true
	}
	return nil, false
}

type MapPlayer struct {
	data *User
	Ins  *aoi.Player
	From *nethub.Client
}

func (m *MapPlayer) FindAroundPlayer() []int64 {
	return m.Ins.GetAroundPlayers()
}

func (p *PlayerManager) InitUser(id int64, from *nethub.Client) (*MapPlayer, error) {
	record, loaded, err := SqlFindUser(id)
	if err != nil {
		return nil, err
	}
	player := aoi.NewPlayer(record.Id)
	if loaded {
		player.EnterMap(record.LastMapId, record.LastPosition[0], record.LastPosition[1], record.LastPosition[2], record.LastYaw)
	} else {
		player.EnterMapByInitPos(1)
	}
	player.SetProperty("from", from)
	ins := &MapPlayer{
		data: record,
		Ins:  player,
		From: from,
	}
	p.players.Store(id, ins)
	return ins, nil
}
