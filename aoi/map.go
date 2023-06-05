package aoi

import (
	"math"
)

type Map struct {
	Id    int
	MinX  float32
	MaxX  float32
	MinY  float32
	MaxY  float32
	gridX float32
	gridY float32

	EnterPos []float32

	xGridCount int
	yGridCount int
	grids      map[int]*Grid
}

func NewMap(mId int, minX, maxX, minY, maxY float32, gridX float32, gridY float32) *Map {
	xGridCount := int(math.Ceil(float64((maxX - minX) / gridX)))
	yGridCount := int(math.Ceil(float64((maxY - minY) / gridY)))
	m := &Map{
		Id:         mId,
		MinX:       minX,
		MaxX:       maxX,
		MinY:       minY,
		MaxY:       maxY,
		gridX:      gridX,
		gridY:      gridY,
		xGridCount: yGridCount,
		yGridCount: yGridCount,
	}
	for y := 0; y < yGridCount; y++ {
		for x := 0; x < xGridCount; x++ {
			gid := y*xGridCount + x
			m.grids[gid] = NewGrid(gid, minX+float32(x)*gridX, minX+(float32(x)+1)*gridX, minY+float32(y)*gridY, minY+(float32(y)+1)*gridY)
		}
	}
	return m
}

func (m *Map) GetGridIdByPos(x, y float32) int {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idy := int(math.Floor(float64((y - m.MinY) / m.gridY)))
	return idx + idy*m.xGridCount
}

func (m *Map) GetGridByPos(x, y float32) *Grid {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idy := int(math.Floor(float64((y - m.MinY) / m.gridY)))
	return m.grids[idx+idy*m.xGridCount]
}

func (m *Map) AddPlayerToGrid(playerId int64, gridId int) {
	m.grids[gridId].AddPlayer(playerId)
}

func (m *Map) RemovePlayerFromGrid(playerId int64, gridId int) {
	m.grids[gridId].RemovePlayer(playerId)
}

func (m *Map) GetAroundGridsByPos(x, y float32) map[int]*Grid {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idy := int(math.Floor(float64((y - m.MinY) / m.gridY)))

	var grids map[int]*Grid
	for i := -1; i <= 1; i++ {
		aroundX := i + idx
		if aroundX < 0 || aroundX > m.xGridCount-1 {
			continue
		}
		for j := -1; j <= 1; j++ {
			aroundY := j + idy
			if aroundY < 0 || aroundY > m.yGridCount-1 {
				continue
			}
			gid := aroundX + aroundY*m.xGridCount
			grids[gid] = m.grids[gid]
		}
	}
	return grids
}
