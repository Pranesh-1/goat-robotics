package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"os"
	"math/rand"

	"goat-backend/simulation"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Presets map[string]Preset `yaml:"presets"`
}

type Preset struct {
	Grid struct {
		Cols     int     `yaml:"cols"`
		Rows     int     `yaml:"rows"`
		SpacingX float64 `yaml:"spacing_x"`
		SpacingY float64 `yaml:"spacing_y"`
	} `yaml:"grid"`
	Robots struct {
		Count int `yaml:"count"`
	} `yaml:"robots"`
	Lanes struct {
		HorizontalBaseSpeed float64 `yaml:"horizontal_base_speed"`
		VerticalBaseSpeed   float64 `yaml:"vertical_base_speed"`
	} `yaml:"lanes"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func buildWorld(ctrl *simulation.Controller, p Preset) {
	graph := simulation.NewGraph()
	cols, rows := p.Grid.Cols, p.Grid.Rows
	spacingX, spacingY := p.Grid.SpacingX, p.Grid.SpacingY

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			id := fmt.Sprintf("N_%d_%d", x, y)
			graph.AddNode(id, float64(x)*spacingX+50, float64(y)*spacingY+50)
			// Mark ALL 4 corners as charging stations (multi-port industrial chargers)
			if (x == 0 || x == cols-1) && (y == 0 || y == rows-1) {
				graph.Nodes[id].IsChargingStation = true
				graph.Nodes[id].Capacity = 2
			}
		}
	}

	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			curr := fmt.Sprintf("N_%d_%d", x, y)
			if x < cols-1 {
				right := fmt.Sprintf("N_%d_%d", x+1, y)
				graph.AddLane(curr+"_"+right, curr, right, p.Lanes.HorizontalBaseSpeed, "high", "normal")
				graph.AddLane(right+"_"+curr, right, curr, p.Lanes.HorizontalBaseSpeed, "high", "normal")
			}
			if y < rows-1 {
				down := fmt.Sprintf("N_%d_%d", x, y+1)
				graph.AddLane(curr+"_"+down, curr, down, p.Lanes.VerticalBaseSpeed, "medium", "narrow")
				graph.AddLane(down+"_"+curr, down, curr, p.Lanes.VerticalBaseSpeed, "medium", "narrow")
			}
		}
	}

	robots := make([]*simulation.Robot, 0, p.Robots.Count)
	
	// Decrease VIP count to exactly 2 for Small, 4 for Large
	maxHigh := 2
	if p.Robots.Count > 10 { maxHigh = 4 }

	for i := 0; i < p.Robots.Count; i++ {
		start := fmt.Sprintf("N_%d_%d", rand.Intn(cols), rand.Intn(rows))
		target := fmt.Sprintf("N_%d_%d", rand.Intn(cols), rand.Intn(rows))
		
		priority := 1
		if i < maxHigh {
			// First few robots are strictly VIP
			priority = 2 
		} else {
			// The rest are randomized Normal/Low
			if rand.Float64() > 0.6 { priority = 0 }
		}
		
		robots = append(robots, simulation.NewRobot("R"+strconv.Itoa(i), start, target, priority))
	}
	
	// Atomically load new graph and robots
	ctrl.BuildWorld(graph, robots)
}

func main() {
	// Parse YAML Config
	yamlData, err := os.ReadFile("config.yaml")
	if err != nil { log.Fatalf("Failed reading config.yaml: %v", err) }
	
	var config Config
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		log.Fatalf("Failed unmarshaling config: %v", err)
	}

	ctrl := simulation.NewController(simulation.NewGraph())
	
	// Boot with small preset by default
	if preset, ok := config.Presets["small"]; ok {
		buildWorld(ctrl, preset)
	}

	// ── Simulation loop at 60 fps ────────────────────────────────────
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			ctrl.Tick(0.016)
		}
	}()

	// ── HTTP API ───────────────────────────────────────────────
	var activePresetName string = "small" // Global tracker for currently loaded preset
	
	http.HandleFunc("/api/preset", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" { return }
		
		presetName := r.URL.Query().Get("name")
		if preset, ok := config.Presets[presetName]; ok {
			buildWorld(ctrl, preset)
			activePresetName = presetName
			fmt.Fprintf(w, `{"status": "ok", "preset": "%s"}`, presetName)
		} else {
			http.Error(w, `{"error": "preset not found"}`, http.StatusNotFound)
		}
	})

	http.HandleFunc("/api/report/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" { return }

		pdfBytes, err := GeneratePDFReport(ctrl, activePresetName)
		if err != nil {
			http.Error(w, `{"error": "Failed to generate report"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=goat_robotics_report.pdf")
		w.Write(pdfBytes)
	})

	http.HandleFunc("/api/estop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			return
		}
		isEmerg := ctrl.ToggleEStop()
		fmt.Fprintf(w, `{"estop": %v}`, isEmerg)
	})

	// ── WebSocket endpoint ───────────────────────────────────────────
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(16 * time.Millisecond) // 60 fps to client
		defer ticker.Stop()

		for range ticker.C {
			data, err := ctrl.Snapshot() // atomic copy, no nested locks
			if err != nil {
				log.Println("snapshot:", err)
				break
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Println("write:", err)
				break
			}
		}
	})

	log.Println("GOAT Robotics backend running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
