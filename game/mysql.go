package main

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	"time"
)

var db *gorm.DB

func InitMysql(dsn string) {
	mysqlDsn := fmt.Sprintf("%v?parseTime=true&loc=Local", dsn)
	fmt.Println("init mysql connection", dsn)
	var err error
	db, err = gorm.Open("mysql", mysqlDsn)
	if err != nil {
		panic(err)
	}
	db.DB().SetConnMaxLifetime(3 * time.Minute)
	//设置最大连接数
	db.DB().SetMaxOpenConns(100)
	//设置闲置连接数
	db.DB().SetMaxIdleConns(20)
	//db.LogMode(true)
}
