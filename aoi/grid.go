package aoi

import "sync"

type Grid struct {
	Id      int
	MinX    float32
	MaxX    float32
	MinZ    float32
	MaxZ    float32
	Players sync.Map
}

func NewGrid(gId int, minX, maxX, minZ, maxZ float32) *Grid {
	return &Grid{
		Id:   gId,
		MinX: minX,
		MaxX: maxX,
		MinZ: minZ,
		MaxZ: maxZ,
	}
}

func (g *Grid) AddPlayer(player *Player) {
	g.Players.Store(player.Id, player)
}

func (g *Grid) RemovePlayer(PlayerId int64) {
	g.Players.Delete(PlayerId)
}
