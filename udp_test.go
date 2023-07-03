package nethub

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"testing"
	"time"
)

func TestBroadcastToAround(t *testing.T) {
	var pkts int
	var byteCount int
	var clients []net.Conn
	var rts [][]byte
	for i := 0; i < 50; i++ {
		conn, err := net.Dial("udp", "192.168.9.86:1234")
		if err != nil {
			fmt.Printf("Dial() failed. Error %s\n", err)
		} else {
			clients = append(clients, conn)
			sn := fmt.Sprintf("sn_%v", i)
			go func() {
				buf := make([]byte, packetBufSize)
				for {
					if len(buf) < UDPPacketSize {
						buf = make([]byte, packetBufSize)
					}
					nbytes, _ := conn.Read(buf)
					_ = buf[:nbytes]
					buf = buf[nbytes:]
				}
			}()

			var data = make(map[string]interface{})
			data["x"] = 10
			data["y"] = 10
			data["z"] = 10
			data["yaw"] = 12
			data["speed"] = 3.3
			data["enu_fl_x"] = 1
			data["enu_fl_y"] = 1
			data["enu_fl_z"] = 1

			data["enu_rl_x"] = 1
			data["enu_rl_y"] = 1
			data["enu_rl_z"] = 1

			data["enu_rr_x"] = 1
			data["enu_rr_y"] = 1
			data["enu_rr_z"] = 1

			data["enu_fr_x"] = 1
			data["enu_fr_y"] = 1
			data["enu_fr_z"] = 1
			data["sn"] = sn

			dataBytes, _ := json.Marshal(data)
			broadcastRpc := RequestPacket{Method: "broadcast_to_around", Params: dataBytes}
			packet := defaultCodec.Marshal(&broadcastRpc, nil)
			rts = append(rts, packet)
		}
	}

	go func() {

		ticker := time.NewTicker(100 * time.Millisecond)
		for {
			//从定时器中获取数据
			_ = <-ticker.C
			for i, client := range clients {
				n, _ := client.Write(rts[i])
				pkts++
				byteCount += n
			}
		}
	}()

	for {
		fmt.Printf("Pkts %d, Bytes %d, rate %f mbps\n",
			pkts, byteCount, float64(byteCount)*8/(1000*1000))
		pkts = 0
		byteCount = 0
		time.Sleep(time.Second)
	}
}

func TestRtMessage(t *testing.T) {
	var clients []*Client
	for i := 1; i < 2; i++ {
		//Conn, err := net.Dial("udp", "139.196.75.44:1234")
		//Session, err := net.Dial("udp", "192.168.9.86:1234")
		//Conn, err := net.Dial("udp", "106.14.223.60:1234")
		var projectId int64 = 53010217439105
		client := DialHubUdp("139.196.75.44:1234", LoginParams{ClientId: "T_001", BucketId: &projectId}, &ClientOptions{
			HeartbeatInterval: 5,
			HeartbeatTimeout:  10,
			WaitTimeout:       10,
			RetryInterval:     5,
		})
		//client := DialHubUdp("127.0.0.1:1234", LoginParams{ClientId: "T_001", BucketId: &projectId})
		clients = append(clients, client)
		//client.SendPacket(&SubscribePacket{Topic: "+/rt_message"})
		//client.SendPacket(&SubscribePacket{Topic: "+/gps_message"})
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	logTicker := time.NewTicker(time.Second)

	var rtMsgMap = make(map[string]interface{})
	msgStr := `{"timestamp": 1671610334.1461706, "network_latency": 100.0, "chair_latency": 0.0, "shovel_vehicle_state": 2, "vehicle_state": 2, "autodrive_state": 2, "longitude": 101.75232644, "latitude": 26.63366599, "altitude": 1116.06578656, "x": -48, "y": -45, "z": 0, "enu_f_l": "[-453.946, -271.892, -8.881]", "enu_r_l": "[-443.215, -270.434, -8.881]", "enu_f_r": "[-454.78, -265.748, -8.881]", "enu_r_r": "[-444.049, -264.291, -8.881]", "yaw": -172.266, "roll": 0.0, "pitch": 0.0, "ax": 1.916, "ay": 0.26, "az": 0.0, "speed": 7.03, "localization_valid": 1, "drive_mode": 2, "goal": 2, "CTE": 0.08, "pressure_f": 8.552, "pressure_r": 8.557, "target_speed": 7.0, "target_gear": 3, "target_steering": -0.013, "target_epb": 0, "target_lift_status": 0, "total_fuel_consumption": 354.652, "odometer": 178.92, "steering": -0.01, "throttle": 38, "brake": 0, "gear": 3, "epb": 0, "lift_angle": 0.0, "engine_speed": 2719, "engine_torque": 120.0, "steering_wheel_torque": 30.0, "steering_wheel_speed": 0.1, "steering_motor_torque": 0.1, "brake_work_mode": 0, "brake_over_heat": 0, "deceleration": -2.055, "brake_pressure_r_r": 0.0, "brake_pressure_r_l": 0.0, "brake_pressure_f_r": 0.0, "brake_pressure_f_l": 0.0, "wheel_speed_r_r": 7.095, "wheel_speed_r_l": 7.095, "wheel_speed_f_r": 7.095, "wheel_speed_f_l": 7.095, "oil": 29.851, "instant_fuel_consumption": 6.8, "engine_oil_pressure": 1.68, "return_oil_temperature": 15.0, "eps_presesure": 89.0, "oil_filter_pressure": 2.0, "ac_switch_state": 0, "airconditioner_state": 0, "smoke_alarm_switch_state": 0, "epb_pressure": 0.76, "left_light": 0, "right_light": 0, "pos_light": 0, "inside_light": 0, "low_beam": 0, "high_beam": 0, "warn_light": 0, "back_light": 0, "horn": 0, "engine_oil_temperature": 50.13, "engine_fuel_temperature": 50.013, "engine_cool_water_temperature": 50.493, "record_path_state": 0}`
	json.Unmarshal([]byte(msgStr), &rtMsgMap)

	pkt := 0
	for {
		select {
		case <-ticker.C:
			for _, client := range clients {
				client.SendPacket(&RequestPacket{Method: "rt_message", Params: []byte(msgStr)})
				pkt++
			}
		case <-logTicker.C:
			fmt.Printf("Pkts %d\n", pkt)
			pkt = 0
		}
	}
}

func TestForSelect(t *testing.T) {

	go func() {
		timeout := time.After(time.Second * time.Duration(5))
		for {
			select {
			case <-time.After(time.Second * time.Duration(1)):
				log.Println("try")
			case <-timeout:
				log.Println("end")
				return
			}
		}
	}()
	select {}
}
