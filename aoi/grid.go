package aoi

import "sync"

type Grid struct {
	Id      int
	MinX    float32
	MaxX    float32
	MinY    float32
	MaxY    float32
	Players sync.Map
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

func (g *Grid) AddPlayer(player *Player) {
	g.Players.Store(player.Id, struct{}{})
}

func (g *Grid) RemovePlayer(PlayerId int64) {
	g.Players.Delete(PlayerId)
}
