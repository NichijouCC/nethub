package aoi

type MapWorld struct {
	maps map[int32]*Map
}

var World *MapWorld

func InitMapWorld(data map[int32]MapData) {
	World = &MapWorld{map[int32]*Map{}}
	for mId, m := range data {
		World.maps[mId] = NewMap(mId, m)
	}
}

type MapData struct {
	MinX, MaxX, MinZ, MaxZ, GridX, GridZ, InitX, InitY, InitZ, InitYaw float32
}

func (m *MapWorld) FindMap(mapId int32) *Map {
	return m.maps[mapId]
}
