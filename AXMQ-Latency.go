package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	// 1. 解析动态参数
	broker := flag.String("h", "172.28.130.87", "Broker IP")
	count := flag.Int("n", 2000, "采样总数")
	interval := flag.Int("i", 10, "发送间隔(ms)")
	flag.Parse()

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:1883", *broker)).
		SetClientID("AXMQ-Prober-Advanced").
		SetCleanSession(true)

	client := mqtt.NewClient(opts)
	fmt.Printf("正在连接 %s 进行延迟探测 (目标样本: %d)...\n", *broker, *count)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "❌ 连接失败: %v\n", token.Error())
		os.Exit(1)
	}

	var mu sync.Mutex
	latencies := make([]time.Duration, 0, *count)
	done := make(chan struct{})

	// 2. 订阅并计算延迟 (增加互斥锁)
	client.Subscribe("latency/test/prober", 0, func(c mqtt.Client, m mqtt.Message) {
		now := time.Now().UnixNano()
		var sentTime int64
		fmt.Sscanf(string(m.Payload()), "%d", &sentTime)

		mu.Lock()
		latencies = append(latencies, time.Duration(now-sentTime))
		if len(latencies) >= *count {
			select {
			case done <- struct{}{}:
			default:
			}
		}
		mu.Unlock()
	})

	// 3. 发送采样包
	go func() {
		for i := 0; i < *count; i++ {
			payload := fmt.Sprintf("%d", time.Now().UnixNano())
			client.Publish("latency/test/prober", 0, false, payload)
			time.Sleep(time.Duration(*interval) * time.Millisecond)
		}
	}()

	// 4. 等待完成或超时
	select {
	case <-done:
		fmt.Println("✅ 采样完成!")
	case <-time.After(30 * time.Second):
		fmt.Printf("⚠️ 采样超时, 仅收到 %d 个包\n", len(latencies))
	}

	// 5. 统计输出
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		l := len(latencies)
		fmt.Printf("\n--- 延迟分布统计 (%d 样本) ---\n", l)
		fmt.Printf("P50: %v\n", latencies[l*50/100])
		fmt.Printf("P95: %v\n", latencies[l*95/100])
		fmt.Printf("P99: %v\n", latencies[l*99/100])
		fmt.Printf("Max: %v\n", latencies[l-1])
	}
	client.Disconnect(250)
}
