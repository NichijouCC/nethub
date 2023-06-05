package aoi

import "time"

var NotifyLeave func(player int64, leave []int64)
var NotifyMove func(player int64, pos PosMessage)

// 15s同步一次,避免通知离开没通知到
var NotifyAroundLive func(player int64, live []int64)

type Player struct {
	Id             int64
	x              float32
	y              float32
	CurrentMap     *Map
	CurrentGrid    *Grid
	AroundGrids    map[int]*Grid
	LastPosMessage PosMessage
}

func (p *Player) EnterMap(mapId int, x float32, y float32, z float32) {
	if p.CurrentMap != nil {
		p.LeaveMap()
	}
	p.CurrentMap = world.maps[mapId]
	p.CurrentGrid = p.CurrentMap.GetGridByPos(x, y)
	p.CurrentGrid.AddPlayer(p.Id)
	p.AroundGrids = p.CurrentMap.GetAroundGridsByPos(x, y)
	if NotifyMove != nil {
		for _, grid := range p.AroundGrids {
			grid.PlayerIds.Range(func(key, value any) bool {
				if key.(int64) != p.Id {
					NotifyMove(key.(int64), PosMessage{
						Position:  []float32{x, y, z},
						Speed:     []float32{0, 0, 0},
						Timestamp: float32(time.Now().UnixMilli()) / 1000,
					})
				}
				return true
			})
		}
	}
}

func (p *Player) LeaveMap() {
	p.CurrentGrid.RemovePlayer(p.Id)
	p.CurrentGrid = nil
	p.CurrentMap = nil
	if NotifyLeave != nil {
		for _, grid := range p.AroundGrids {
			grid.PlayerIds.Range(func(key, value any) bool {
				if key.(int64) != p.Id {
					NotifyLeave(key.(int64), []int64{p.Id})
				}
				return true
			})
		}
	}
}

func (p *Player) UpdatePos(msg PosMessage) {
	if msg.Position[0]-p.x < 0.1 && msg.Position[1]-p.y < 0.1 { //微小移动忽略
		return
	}
	p.LastPosMessage = msg
	p.x = msg.Position[0]
	p.y = msg.Position[1]
	newGrid := p.CurrentMap.GetGridByPos(p.x, p.y)
	if newGrid != p.CurrentGrid {
		oldGrids := p.AroundGrids
		p.AroundGrids = p.CurrentMap.GetAroundGridsByPos(p.x, p.y)
		//退出的格子，通知自己其他人消失消息，通知其中其他人自己离开消息
		if NotifyLeave != nil {
			var leaves []int64
			for gid, _ := range oldGrids {
				if grid, ok := p.AroundGrids[gid]; !ok {
					grid.PlayerIds.Range(func(key, value any) bool {
						NotifyLeave(key.(int64), []int64{p.Id})
						leaves = append(leaves, key.(int64))
						return true
					})
				}
			}
			NotifyLeave(p.Id, leaves)
		}
		//通知周边玩家,自身位置/加速度/
		if NotifyMove != nil {
			for _, grid := range p.AroundGrids {
				grid.PlayerIds.Range(func(key, value any) bool {
					if key.(int64) != p.Id {
						NotifyMove(key.(int64), msg)
					}
					return true
				})
			}
		}
	}
}

func (p *Player) TickNotifyAroundLive() {
	var lives []int64
	for _, grid := range p.AroundGrids {
		grid.PlayerIds.Range(func(key, value any) bool {
			if key.(int64) != p.Id {
				lives = append(lives, key.(int64))
			}
			return true
		})
	}
	if NotifyAroundLive != nil {
		NotifyAroundLive(p.Id, lives)
	}
}

type PosMessage struct {
	Position  []float32 `json:"position"`
	Speed     []float32 `json:"speed"`
	Timestamp float32   `json:"timestamp"`
}
