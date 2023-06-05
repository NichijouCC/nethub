package aoi

type World struct {
	maps map[int]*Map
}

var world *World

func InitWorld(data map[int]mapData) {
	world = &World{map[int]*Map{}}
	for mId, m := range data {
		world.maps[mId] = NewMap(mId, m.minX, m.maxX, m.minY, m.maxY, m.gridX, m.gridY)
	}
}

type mapData struct {
	minX, maxX, minY, maxY, gridX, gridY float32
}
