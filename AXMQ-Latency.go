package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	cid := "AXMQ-Latency-Prober-Final"
	opts := mqtt.NewClientOptions().
		AddBroker("tcp://172.28.130.87:1883").
		SetClientID(cid)

	opts.SetDialer(&net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP("172.28.130.109"),
		},
		Timeout: 10 * time.Second,
	})

	client := mqtt.NewClient(opts)
	fmt.Println("正在 1M 连接背景下尝试探测...")
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法连接 (可能是 1M 负载下 IP 端口全满): %v\n", token.Error())
		os.Exit(1)
	}

	latencies := []time.Duration{}
	msgCount := 300
	receivedChan := make(chan bool)

	client.Subscribe("latency/test/prober", 0, func(c mqtt.Client, m mqtt.Message) {
		var sentTime int64
		fmt.Sscanf(string(m.Payload()), "%d", &sentTime)
		latencies = append(latencies, time.Duration(time.Now().UnixNano()-sentTime))
		if len(latencies) >= msgCount {
			select {
			case receivedChan <- true:
			default:
			}
		}
	})

	for i := 0; i < msgCount; i++ {
		payload := fmt.Sprintf("%d", time.Now().UnixNano())
		client.Publish("latency/test/prober", 0, false, payload)
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case <-receivedChan:
		fmt.Println("✅ 1M 负载背景采样完成!")
	case <-time.After(15 * time.Second):
		fmt.Printf("⚠️ 采样超时, 收到回包: %d\n", len(latencies))
	}

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		fmt.Printf("\n--- 1M 连接负载背景下的延迟分布 ---\n")
		fmt.Printf("P50: %v\n", latencies[len(latencies)*50/100])
		fmt.Printf("P95: %v\n", latencies[len(latencies)*95/100])
		fmt.Printf("P99: %v\n", latencies[len(latencies)*99/100])
		fmt.Printf("Max: %v\n", latencies[len(latencies)-1])
	}
	client.Disconnect(250)
}
