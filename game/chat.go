package main

import (
	"errors"
	"github.com/NichijouCC/nethub"
	"github.com/NichijouCC/nethub/game/pb"
	"github.com/golang/protobuf/proto"
	"sync"
)

var chatMgr *ChatManager

func InitChatMgr() {
	chatMgr = &ChatManager{}
}

type ChatManager struct {
	Users  sync.Map
	Groups sync.Map
}

func (c *ChatManager) FindGroup(groupId int64) (*ChatGroup, bool) {
	value, loaded := c.Groups.Load(groupId)
	if loaded {
		return value.(*ChatGroup), true
	}
	return nil, false
}

func (c *ChatManager) FindUser(userId int64) (*ChatUser, bool) {
	value, loaded := c.Users.Load(userId)
	if loaded {
		return value.(*ChatUser), true
	}
	return nil, false
}

type ChatGroup struct {
	GroupId int64
	Users   sync.Map
}

type ChatUser struct {
	UserId   int64
	UserName string
	From     *nethub.Client
}

func (c *ChatUser) ChatInWorld(req pb.WorldChatReq) {
	req.FromUserId = c.UserId
	req.FromUserName = c.UserName
	data, _ := proto.Marshal(&req)
	chatMgr.Users.Range(func(key, value any) bool {
		user := value.(*ChatUser)
		if user != c {
			user.From.Request("31", data)
		}
		return true
	})
}

func (c *ChatUser) ChatInGroup(req pb.GroupChatReq) error {
	group, ok := chatMgr.FindGroup(req.ToGroupId)
	if !ok {
		return errors.New("无法找到group")
	}
	req.FromUserId = c.UserId
	req.FromUserName = c.UserName
	data, _ := proto.Marshal(&req)
	group.Users.Range(func(key, value any) bool {
		user := value.(*ChatUser)
		if user != c {
			user.From.Request("32", data)
		}
		return true
	})
	return nil
}

func (c *ChatUser) ChatToUser(req pb.PrivateChatReq) error {
	user, ok := chatMgr.FindUser(req.ToUserId)
	if !ok {
		return errors.New("无法找到user")
	}
	req.FromUserId = c.UserId
	req.FromUserName = c.UserName
	data, _ := proto.Marshal(&req)
	user.From.Request("33", data)
	return nil
}

func (c *ChatUser) ChatToLocal(req pb.LocalChatReq) error {
	player, ok := playerMgr.FindPlayer(c.UserId)
	if !ok {
		return errors.New("无法找到player")
	}
	req.FromUserId = c.UserId
	req.FromUserName = c.UserName
	data, _ := proto.Marshal(&req)

	players := player.FindAroundPlayer()
	for _, playerId := range players {
		user, ok := chatMgr.FindUser(playerId)
		if ok {
			user.From.Request("34", data)
		}
	}
	return nil
}

func (c *ChatManager) InitUser(userId int64, from *nethub.Client) (*ChatUser, error) {
	return &ChatUser{}, nil
}
