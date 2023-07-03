package main

import (
	"errors"
)

type User struct {
	UserId int64 `gorm:"primary_key"`
	//
	LastMapId int32
	//位置
	LastPositionX float32
	LastPositionY float32
	LastPositionZ float32
	//朝向
	LastYaw float32
	//积分
	Score int64
}

func SqlFindUser(userId int64) (*User, bool, error) {
	var record User
	result := db.Table("users").Where("user_id = ?", userId).Take(&record)
	if result.RecordNotFound() {
		return nil, false, nil
	}
	if result.Error != nil {
		return nil, false, errors.New("sql查询出错")
	}
	return &record, true, nil
}
