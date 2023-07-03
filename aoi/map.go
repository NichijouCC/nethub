package aoi

import (
	"math"
)

type Map struct {
	Id                           int32
	MinX                         float32
	MaxX                         float32
	MinZ                         float32
	MaxZ                         float32
	gridX                        float32
	gridZ                        float32
	initX, initY, initZ, initYaw float32
	EnterPos                     []float32

	xGridCount int
	yGridCount int
	grids      map[int]*Grid
}

func NewMap(mId int32, data MapData) *Map {
	xGridCount := int(math.Ceil(float64((data.MaxX - data.MinX) / data.GridX)))
	zGridCount := int(math.Ceil(float64((data.MaxZ - data.MinZ) / data.GridZ)))
	m := &Map{
		Id:         mId,
		MinX:       data.MinX,
		MaxX:       data.MaxX,
		MinZ:       data.MinZ,
		MaxZ:       data.MaxZ,
		gridX:      data.GridX,
		gridZ:      data.GridZ,
		initX:      data.InitX,
		initY:      data.InitY,
		initZ:      data.InitZ,
		initYaw:    data.InitYaw,
		xGridCount: zGridCount,
		yGridCount: zGridCount,
		grids:      map[int]*Grid{},
	}
	for z := 0; z < zGridCount; z++ {
		for x := 0; x < xGridCount; x++ {
			gid := z*xGridCount + x
			m.grids[gid] = NewGrid(gid, data.MinX+float32(x)*data.GridZ, data.MinX+(float32(x)+1)*data.GridX, data.MinZ+float32(z)*data.GridZ, data.MinZ+(float32(z)+1)*data.GridZ)
		}
	}
	return m
}

func (m *Map) GetGridIdByPos(x, z float32) int {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idy := int(math.Floor(float64((z - m.MinZ) / m.gridZ)))
	return idx + idy*m.xGridCount
}

func (m *Map) GetGridByPos(x, z float32) *Grid {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idz := int(math.Floor(float64((z - m.MinZ) / m.gridZ)))
	return m.grids[idx+idz*m.xGridCount]
}

func (m *Map) AddPlayerToGrid(player *Player, gridId int) {
	m.grids[gridId].AddPlayer(player)
}

func (m *Map) RemovePlayerFromGrid(playerId int64, gridId int) {
	m.grids[gridId].RemovePlayer(playerId)
}

func (m *Map) GetAroundGridsByPos(x, z float32) map[int]*Grid {
	idx := int(math.Floor(float64((x - m.MinX) / m.gridX)))
	idz := int(math.Floor(float64((z - m.MinZ) / m.gridZ)))

	var grids = make(map[int]*Grid)
	for i := -1; i <= 1; i++ {
		aroundX := i + idx
		if aroundX < 0 || aroundX > m.xGridCount-1 {
			continue
		}
		for j := -1; j <= 1; j++ {
			aroundZ := j + idz
			if aroundZ < 0 || aroundZ > m.yGridCount-1 {
				continue
			}
			gid := aroundX + aroundZ*m.xGridCount
			grids[gid] = m.grids[gid]
		}
	}
	return grids
}
