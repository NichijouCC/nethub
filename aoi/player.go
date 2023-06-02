package aoi

type Player struct {
	Id          int64
	x           float32
	y           float32
	World       *World
	CurrentMap  *Map
	CurrentGrid *Grid
}

func (p *Player) EnterMap(mapId int, x float32, y float32) {
	if p.CurrentMap != nil {
		p.LeaveMap()

	}
	p.CurrentMap = p.World.Maps[mapId]
	p.CurrentGrid = p.CurrentMap.GetGridByPos(x, y)
	p.CurrentGrid.AddPlayer(p.Id)
}

func (p *Player) LeaveMap() {
	p.CurrentGrid.RemovePlayer(p.Id)
	p.CurrentMap = nil
	p.CurrentGrid = nil
}

func (p *Player) UpdatePos(msg PosMessage) {
	if msg.Position[0]-p.x < 0.1 && msg.Position[1]-p.y < 0.1 { //微小移动忽略
		return
	}
	p.x = msg.Position[0]
	p.y = msg.Position[1]
	newGrid := p.CurrentMap.GetGridByPos(p.x, p.y)
	if newGrid != p.CurrentGrid {
		//grid修改了,在其他人的视野里消失，或者出现

	}
	//通知周边玩家,自身位置/加速度/
	p.CurrentMap.AroundPlayersRange(p.x, p.y, func(playerId int64) {
		//通知

	})
}

type PosMessage struct {
	Position     []float32
	Acceleration []float32
	Timestamp    float32
}
