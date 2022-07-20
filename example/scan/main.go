package main

import (
	"fmt"
	"github.com/gosown/scan"
	"net"
	"time"
)

type Lite struct {
	LiteUserField  int32 `scan:"lite_field"`
	LiteUserFieldd int32 `scan:"lite_fieldd"`
}

type Num struct {
	One int `scan:"one"`
	Two int `scan:"two"`
}

type Area struct {
	UserID   int    `scan:"id"`
	Province string `scan:"province"`
	City     string `scan:"city"`
	Num      *Num   `scan:"num"`
}

type UserInfo struct {
	Area   *Area    `scan:"area"`
	IP     *scan.IP `scan:"ip"`
	UserID int64    `scan:"id"`
	*Lite
	CreatedAt *scan.Time `scan:"created_at" layout:"2006-01-02 15:04:05"`
}

func main() {
	var userInfo UserInfo
	userInfo.IP = new(scan.IP)
	userInfo.Lite = new(Lite)
	userInfo.Area = new(Area)
	userInfo.Area.Num = new(Num)
	userInfo.CreatedAt = new(scan.Time)
	source := map[string]interface{}{
		"id":          "110",
		"lite_field":  "666",
		"lite_fieldd": "888",
		"area": map[string]interface{}{
			"id":       "119",
			"province": "sichuan",
			"city":     "chengdu",
			"num": map[string]interface{}{
				"one": "1",
				"two": "2",
			},
		},
		"ip":         net.ParseIP("127.0.0.1").String(),
		"created_at": time.Now().Format("2006-01-02 15:04:05"),
	}

	err := scan.ScanStruct(source, &userInfo)
	if err != nil {
		panic(err)
	}
	fmt.Println(userInfo.Area.Num)
	fmt.Println(userInfo.IP)
	fmt.Println(time.Time(*userInfo.CreatedAt))
	fmt.Println(userInfo.UserID)
}
