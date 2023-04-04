package nethub

import (
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"
)

func TestTcpSendRtMessage(t *testing.T) {
	msgStr := `{"timestamp": 1671610334.1461706, "network_latency": 100.0, "chair_latency": 0.0, "shovel_vehicle_state": 2, "vehicle_state": 2, "autodrive_state": 2, "longitude": 101.75232644, "latitude": 26.63366599, "altitude": 1116.06578656, "x": -48, "y": -45, "z": 0, "enu_f_l": "[-453.946, -271.892, -8.881]", "enu_r_l": "[-443.215, -270.434, -8.881]", "enu_f_r": "[-454.78, -265.748, -8.881]", "enu_r_r": "[-444.049, -264.291, -8.881]", "yaw": -172.266, "roll": 0.0, "pitch": 0.0, "ax": 1.916, "ay": 0.26, "az": 0.0, "speed": 7.03, "localization_valid": 1, "drive_mode": 2, "goal": 2, "CTE": 0.08, "pressure_f": 8.552, "pressure_r": 8.557, "target_speed": 7.0, "target_gear": 3, "target_steering": -0.013, "target_epb": 0, "target_lift_status": 0, "total_fuel_consumption": 354.652, "odometer": 178.92, "steering": -0.01, "throttle": 38, "brake": 0, "gear": 3, "epb": 0, "lift_angle": 0.0, "engine_speed": 2719, "engine_torque": 120.0, "steering_wheel_torque": 30.0, "steering_wheel_speed": 0.1, "steering_motor_torque": 0.1, "brake_work_mode": 0, "brake_over_heat": 0, "deceleration": -2.055, "brake_pressure_r_r": 0.0, "brake_pressure_r_l": 0.0, "brake_pressure_f_r": 0.0, "brake_pressure_f_l": 0.0, "wheel_speed_r_r": 7.095, "wheel_speed_r_l": 7.095, "wheel_speed_f_r": 7.095, "wheel_speed_f_l": 7.095, "oil": 29.851, "instant_fuel_consumption": 6.8, "engine_oil_pressure": 1.68, "return_oil_temperature": 15.0, "eps_presesure": 89.0, "oil_filter_pressure": 2.0, "ac_switch_state": 0, "airconditioner_state": 0, "smoke_alarm_switch_state": 0, "epb_pressure": 0.76, "left_light": 0, "right_light": 0, "pos_light": 0, "inside_light": 0, "low_beam": 0, "high_beam": 0, "warn_light": 0, "back_light": 0, "horn": 0, "engine_oil_temperature": 50.13, "engine_fuel_temperature": 50.013, "engine_cool_water_temperature": 50.493, "record_path_state": 0}`
	var clients []*TcpConn
	var rts [][]byte
	for i := 1; i < 4; i++ {
		//Session, err := net.Dial("udp", "139.196.75.44:1234")
		//Session, err := net.Dial("udp", "192.168.9.86:1234")
		conn, err := DialTcp("127.0.0.1:1235")
		if err != nil {
			fmt.Printf("Dial() failed. Error %s\n", err)
		} else {
			clients = append(clients, conn)
			sn := fmt.Sprintf("T_%03d", i)
			var rtMsgMap = make(map[string]interface{})
			json.Unmarshal([]byte(msgStr), &rtMsgMap)
			rtMsgMap["sn"] = sn
			rtBytes, _ := json.Marshal(rtMsgMap)

			//request := RpcRequest{Method: "rt_message", Params: string(rtBytes)}

			//var projectId int64 = 53010217439105
			request := RequestPacket{Method: "rt_message", Params: rtBytes}
			packet := packetCoder.marshal(&request)
			rts = append(rts, packet)
			conn.StartReadWrite()
		}
	}
	var pkts int

	ticker := time.NewTicker(100 * time.Millisecond)
	logTicker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			for i, client := range clients {
				err := client.SendMessage(rts[i])
				if err != nil {
					log.Println("error", err)
				}
				pkts++
			}
		case <-logTicker.C:
			fmt.Printf("Pkts %d\n",
				pkts)
			pkts = 0
		}
	}
}
