package aoi

import (
	"math"
)

type Map struct {
	Id                           int32
	MinX                         float32
	MaxX                         float32
	MinY                         float32
	MaxY                         float32
	gridX                        float32
	gridY                        float32
	initX, initY, initZ, initYaw float32
	EnterPos                     []float32

	xGridCount int
	yGridCount int
	grids      map[int]*Grid
}

func NewMap(mId int32, data MapData) *Map {
	xGridCount := int(math.Ceil(float64((data.maxX - data.minX) / data.gridX)))
	yGridCount := int(math.Ceil(float64((data.maxY - data.minY) / data.gridY)))
	m := &Map{
		Id:         mId,
		MinX:       data.minX,
		MaxX:       data.maxX,
		MinY:       data.minY,
		MaxY:       data.maxY,
		gridX:      data.gridX,
		gridY:      data.gridY,
		initX:      data.initX,
		initY:      data.initY,
		initZ:      data.initZ,
		initYaw:    data.initYaw,
		xGridCount: yGridCount,
		yGridCount: yGridCount,
	}
	for y := 0; y < yGridCount; y++ {
		for x := 0; x < xGridCount; x++ {
			gid := y*xGridCount + x
			m.grids[gid] = NewGrid(gid, data.minX+float32(x)*data.gridY, data.minX+(float32(x)+1)*data.gridX, data.minY+float32(y)*data.gridY, data.minY+(float32(y)+1)*data.gridY)
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

func (m *Map) AddPlayerToGrid(player *Player, gridId int) {
	m.grids[gridId].AddPlayer(player)
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
