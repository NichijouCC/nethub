package aoi

import "sync"

type Grid struct {
	Id        int
	MinX      float32
	MaxX      float32
	MinY      float32
	MaxY      float32
	PlayerIds sync.Map
}

func NewGrid(gId int, minX, maxX, minY, maxY float32) *Grid {
	return &Grid{
		Id:   gId,
		MinX: minX,
		MaxX: maxX,
		MinY: minY,
		MaxY: maxY,
	}
}

func (g *Grid) AddPlayer(playerId int64) {
	g.PlayerIds.Store(playerId, struct{}{})
}

func (g *Grid) RemovePlayer(PlayerId int64) {
	g.PlayerIds.Delete(PlayerId)
}

func (g *Grid) GetPlayers() []int64 {
	var players []int64
	g.PlayerIds.Range(func(key, value any) bool {
		players = append(players, key.(int64))
		return true
	})
	return players
}
