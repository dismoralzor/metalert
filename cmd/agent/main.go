package main

import (
	"fmt"
	"log"
	"maps"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/dismoralzor/metalert/internal/model"
)

// poll и report работают в разных горутинах, поэтому доступ к полям под мьютексом.
type agent struct {
	mu        sync.Mutex
	gauges    map[string]float64
	pollCount int64
}

func newAgent() *agent {
	return &agent{gauges: make(map[string]float64)}
}

func (a *agent) poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	a.mu.Lock()
	defer a.mu.Unlock()

	a.gauges["Alloc"] = float64(m.Alloc)
	a.gauges["BuckHashSys"] = float64(m.BuckHashSys)
	a.gauges["Frees"] = float64(m.Frees)
	a.gauges["GCCPUFraction"] = m.GCCPUFraction
	a.gauges["GCSys"] = float64(m.GCSys)
	a.gauges["HeapAlloc"] = float64(m.HeapAlloc)
	a.gauges["HeapIdle"] = float64(m.HeapIdle)
	a.gauges["HeapInuse"] = float64(m.HeapInuse)
	a.gauges["HeapObjects"] = float64(m.HeapObjects)
	a.gauges["HeapReleased"] = float64(m.HeapReleased)
	a.gauges["HeapSys"] = float64(m.HeapSys)
	a.gauges["LastGC"] = float64(m.LastGC)
	a.gauges["Lookups"] = float64(m.Lookups)
	a.gauges["MCacheInuse"] = float64(m.MCacheInuse)
	a.gauges["MCacheSys"] = float64(m.MCacheSys)
	a.gauges["MSpanInuse"] = float64(m.MSpanInuse)
	a.gauges["MSpanSys"] = float64(m.MSpanSys)
	a.gauges["Mallocs"] = float64(m.Mallocs)
	a.gauges["NextGC"] = float64(m.NextGC)
	a.gauges["NumForcedGC"] = float64(m.NumForcedGC)
	a.gauges["NumGC"] = float64(m.NumGC)
	a.gauges["OtherSys"] = float64(m.OtherSys)
	a.gauges["PauseTotalNs"] = float64(m.PauseTotalNs)
	a.gauges["StackInuse"] = float64(m.StackInuse)
	a.gauges["StackSys"] = float64(m.StackSys)
	a.gauges["Sys"] = float64(m.Sys)
	a.gauges["TotalAlloc"] = float64(m.TotalAlloc)
	// math/rand с Go 1.20+ сеется случайно сам, явный Seed не нужен.
	a.gauges["RandomValue"] = rand.Float64()

	a.pollCount++
}

// Снимок под мьютексом отдельно от самой отправки - не держать лок на время HTTP-запросов.
func (a *agent) report(client *http.Client, addr string) {
	a.mu.Lock()
	gaugesSnapshot := make(map[string]float64, len(a.gauges))
	maps.Copy(gaugesSnapshot, a.gauges)
	pollCount := a.pollCount
	a.mu.Unlock()

	for name, value := range gaugesSnapshot {
		sendMetric(client, addr, models.Gauge, name, strconv.FormatFloat(value, 'f', -1, 64))
	}

	// Сервер накапливает counter через += (UpdateCounter), поэтому шлём прирост
	// с прошлой отправки, а не общий счётчик с начала работы агента. Вычитаем
	// его из pollCount только после подтверждённой отправки - если POST не дойдёт,
	// прирост останется в pollCount и уйдёт со следующим отчётом, а не потеряется.
	if sendMetric(client, addr, models.Counter, "PollCount", strconv.FormatInt(pollCount, 10)) {
		a.mu.Lock()
		a.pollCount -= pollCount
		a.mu.Unlock()
	}
}

func sendMetric(client *http.Client, addr, metricType, name, value string) bool {
	url := fmt.Sprintf("http://%s/update/%s/%s/%s", addr, metricType, name, value)

	resp, err := client.Post(url, "text/plain", nil)
	if err != nil {
		log.Printf("send %s %s: %v", metricType, name, err)
		return false
	}
	defer resp.Body.Close()
	return true
}

func main() {
	cfg := parseFlags()

	a := newAgent()
	client := &http.Client{Timeout: 5 * time.Second}

	pollTicker := time.NewTicker(cfg.pollInterval)
	reportTicker := time.NewTicker(cfg.reportInterval)

	go func() {
		for range pollTicker.C {
			a.poll()
		}
	}()

	go func() {
		for range reportTicker.C {
			a.report(client, cfg.addr)
		}
	}()

	select {}
}
