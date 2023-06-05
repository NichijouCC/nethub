package aoi

import (
	"sync"
	"time"
)

var NotifyLeave func(player *Player, leave []int64)
var NotifyMove func(player *Player, pos PosMessage)

// 15s同步一次,避免通知离开没通知到
var NotifyAroundLive func(player *Player, live []int64)

type Player struct {
	Id          int64
	x           float32
	y           float32
	z           float32
	yaw         float32
	CurrentMap  *Map
	CurrentGrid *Grid
	AroundGrids map[int]*Grid
	properties  sync.Map
}

func NewPlayer(id int64) *Player {
	return &Player{Id: id}
}

func (p *Player) SetProperty(key interface{}, value interface{}) {
	p.properties.Store(key, value)
}
func (p *Player) GetProperty(key interface{}) (interface{}, bool) {
	return p.properties.Load(key)
}

func (p *Player) GetPosition() []float32 {
	return []float32{p.x, p.y, p.z}
}

func (p *Player) GetYaw() float32 {
	return p.yaw
}

func (p *Player) EnterMapByInitPos(mapId int32) {
	pmap := World.maps[mapId]
	x := pmap.initX
	y := pmap.initY
	z := pmap.initZ
	yaw := pmap.initYaw
	p.EnterMap(mapId, x, y, z, yaw)
}

func (p *Player) EnterMap(mapId int32, x float32, y float32, z float32, yaw float32) {
	if p.CurrentMap != nil {
		p.LeaveMap()
	}
	p.CurrentMap = World.maps[mapId]
	p.CurrentGrid = p.CurrentMap.GetGridByPos(x, y)
	p.CurrentGrid.AddPlayer(p)
	p.AroundGrids = p.CurrentMap.GetAroundGridsByPos(x, y)
	if NotifyMove != nil {
		for _, grid := range p.AroundGrids {
			grid.Players.Range(func(key, value any) bool {
				if key.(int64) != p.Id {
					NotifyMove(value.(*Player), PosMessage{
						Position:  []float32{x, y, z},
						Yaw:       yaw,
						Speed:     0,
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
			grid.Players.Range(func(key, value any) bool {
				if key.(int64) != p.Id {
					NotifyLeave(value.(*Player), []int64{p.Id})
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
	p.x = msg.Position[0]
	p.y = msg.Position[1]
	p.z = msg.Position[2]
	p.yaw = msg.Yaw
	newGrid := p.CurrentMap.GetGridByPos(p.x, p.y)
	if newGrid != p.CurrentGrid {
		oldGrids := p.AroundGrids
		p.AroundGrids = p.CurrentMap.GetAroundGridsByPos(p.x, p.y)
		//退出的格子，通知自己其他人消失消息，通知其中其他人自己离开消息
		if NotifyLeave != nil {
			var leaves []int64
			for gid, _ := range oldGrids {
				if grid, ok := p.AroundGrids[gid]; !ok {
					grid.Players.Range(func(key, value any) bool {
						NotifyLeave(value.(*Player), []int64{p.Id})
						leaves = append(leaves, key.(int64))
						return true
					})
				}
			}
			NotifyLeave(p, leaves)
		}
		//通知周边玩家,自身位置/加速度/
		if NotifyMove != nil {
			for _, grid := range p.AroundGrids {
				grid.Players.Range(func(key, value any) bool {
					if key.(int64) != p.Id {
						NotifyMove(value.(*Player), msg)
					}
					return true
				})
			}
		}
	}
}

func (p *Player) TickNotifyAroundLive() {
	var lives = p.GetAroundPlayers()
	if NotifyAroundLive != nil {
		NotifyAroundLive(p, lives)
	}
}

func (p *Player) GetAroundPlayers() []int64 {
	var lives []int64
	for _, grid := range p.AroundGrids {
		grid.Players.Range(func(key, value any) bool {
			if key.(int64) != p.Id {
				lives = append(lives, key.(int64))
			}
			return true
		})
	}
	return lives
}

type PosMessage struct {
	Position  []float32 `json:"position"`
	Yaw       float32   `json:"yaw"`
	Speed     float32   `json:"speed"`
	Timestamp float32   `json:"timestamp"`
}
