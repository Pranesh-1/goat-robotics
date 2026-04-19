package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
)

type Controller struct {
	Graph             *Graph
	Robots            map[string]*Robot
	transitLocks      map[string][]string
	stuckTicks        map[string]int
	deadTicks         map[string]int
	chargerOccupancy  map[string]int    // nodeID → how many robots currently docked there
	chargingIntent    map[string]int    // nodeID → how many robots en route to charge there
	Hazards           map[string]float64 // nodeID -> time remaining (spills/debris)
	EStop             bool
	TimeElapsed       float64
	GoalsReached      int
	TotalDelay        float64
	EventLog          []string
	mu                sync.Mutex
}

func NewController(g *Graph) *Controller {
	return &Controller{
		Graph:            g,
		Robots:           make(map[string]*Robot),
		transitLocks:     make(map[string][]string),
		stuckTicks:       make(map[string]int),
		deadTicks:        make(map[string]int),
		chargerOccupancy: make(map[string]int),
		chargingIntent:   make(map[string]int),
		Hazards:          make(map[string]float64),
		EventLog:         make([]string, 0, 50),
	}
}

func (c *Controller) logEvent(msg string) {
	if len(c.EventLog) >= 50 {
		c.EventLog = c.EventLog[1:] // Drop oldest
	}
	c.EventLog = append(c.EventLog, msg)
}

func (c *Controller) AddRobot(r *Robot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Robots[r.ID] = r
	// Idle robots MUST hold the lock for their current node so no one crashes into them
	c.transitLocks[r.CurrentNode] = append(c.transitLocks[r.CurrentNode], r.ID)
}

func (c *Controller) ToggleEStop() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.EStop = !c.EStop
	return c.EStop
}

func (c *Controller) Reset(newGraph *Graph) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Graph = newGraph
	c.Robots = make(map[string]*Robot)
	c.transitLocks = make(map[string][]string)
	c.stuckTicks = make(map[string]int)
	c.deadTicks = make(map[string]int)
	c.chargerOccupancy = make(map[string]int)
	c.chargingIntent = make(map[string]int)
	c.GoalsReached = 0
	c.TotalDelay = 0.0
	c.EStop = false
	c.EventLog = make([]string, 0, 50)
	c.logEvent("[SYS] Simulation world reset")
}

func (c *Controller) BuildWorld(newGraph *Graph, robots []*Robot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Graph = newGraph
	c.Robots = make(map[string]*Robot)
	c.transitLocks = make(map[string][]string)
	c.stuckTicks = make(map[string]int)
	c.deadTicks = make(map[string]int)
	c.chargerOccupancy = make(map[string]int)
	c.chargingIntent = make(map[string]int)
	c.Hazards = make(map[string]float64)
	c.GoalsReached = 0
	c.TotalDelay = 0.0
	c.TimeElapsed = 0.0
	c.EStop = false
	c.EventLog = make([]string, 0, 50)
	c.logEvent("[SYS] World loaded — " + fmt.Sprintf("%d nodes, %d robots", len(newGraph.Nodes), len(robots)))
	for _, r := range robots {
		c.Robots[r.ID] = r
		c.transitLocks[r.CurrentNode] = append(c.transitLocks[r.CurrentNode], r.ID) // initially lock spawn nodes
	}
}
type DetailedMetrics struct {
	TotalRobots  int
	VIPCount     int
	GoalsReached int
	TotalDelay   float64
	ActiveHazards int
	Throughput   float64
}

func (c *Controller) GetMetrics() DetailedMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	vip := 0
	for _, r := range c.Robots {
		if r.Priority == 2 { vip++ }
	}
	throughput := 0.0
	if c.TimeElapsed > 0 { throughput = (float64(c.GoalsReached) / c.TimeElapsed) * 60.0 }
	
	return DetailedMetrics{
		TotalRobots:  len(c.Robots),
		VIPCount:     vip,
		GoalsReached: c.GoalsReached,
		TotalDelay:   c.TotalDelay,
		ActiveHazards: len(c.Hazards),
		Throughput:   throughput,
	}
}

// Snapshot returns an atomic deep copy for the websocket sender.
func (c *Controller) Snapshot() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	type State struct {
		Nodes   []*Node  `json:"nodes"`
		Lanes   []*Lane  `json:"lanes"`
		Robots  []*Robot `json:"robots"`
		Hazards []string `json:"hazards"`
		EventLog []string `json:"eventLog"`
		Metrics struct {
			Throughput float64 `json:"throughput"`
			Delay      float64 `json:"delay"`
		} `json:"metrics"`
	}
	nodes := make([]*Node, 0, len(c.Graph.Nodes))
	for _, n := range c.Graph.Nodes { cp := *n; nodes = append(nodes, &cp) }
	lanes := make([]*Lane, 0, len(c.Graph.Lanes))
	for _, l := range c.Graph.Lanes {
		cp := *l
		cp.Occupants = append([]string{}, l.Occupants...)
		lanes = append(lanes, &cp)
	}
	robots := make([]*Robot, 0, len(c.Robots))
	for _, r := range c.Robots { cp := *r; cp.Path = append([]string{}, r.Path...); robots = append(robots, &cp) }
	
	// Sort by ID to ensure deterministic tether rendering on frontend
	sort.Slice(robots, func(i, j int) bool {
		return robots[i].ID < robots[j].ID
	})
	
	eventLog := append([]string{}, c.EventLog...)
	
	hazards := make([]string, 0, len(c.Hazards))
	for h := range c.Hazards { hazards = append(hazards, h) }
	
	throughput := 0.0
	if c.TimeElapsed > 0 { throughput = (float64(c.GoalsReached) / c.TimeElapsed) * 60.0 }
	
	return json.Marshal(State{
		Nodes:    nodes,
		Lanes:    lanes,
		Robots:   robots,
		Hazards:  hazards,
		EventLog: eventLog,
		Metrics: struct {
			Throughput float64 `json:"throughput"`
			Delay      float64 `json:"delay"`
		}{
			Throughput: throughput,
			Delay:      c.TotalDelay,
		},
	})
}

// Tick advances all robots then resolves deadlocks.
func (c *Controller) Tick(dt float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.EStop {
		for _, r := range c.Robots {
			r.Speed = 0
			r.Status = "emergency"
		}
		if len(c.EventLog) == 0 || c.EventLog[len(c.EventLog)-1] != "[ESTOP] Emergency stop ACTIVE" {
			c.logEvent("[ESTOP] Emergency stop ACTIVE")
		}
		return
	}
	
	c.TimeElapsed += dt
	
	// Process Random Dynamic Hazards Update
	for h, timeLeft := range c.Hazards {
		c.Hazards[h] = timeLeft - dt
		if c.Hazards[h] <= 0 {
			delete(c.Hazards, h)
			c.logEvent("[SYS] HAZARD CLEARED at " + h)
		}
	}
	// 0.05% chance per tick to spawn a hazard natively (if none exist)
	if len(c.Hazards) < 2 && rand.Float64() < 0.0005 {
		keys := make([]string, 0, len(c.Graph.Nodes))
		for k, n := range c.Graph.Nodes { 
			if !n.IsChargingStation && len(c.transitLocks[k]) == 0 { keys = append(keys, k) } 
		}
		if len(keys) > 0 {
			h := keys[rand.Intn(len(keys))]
			c.Hazards[h] = 8.0 // lasts 8 seconds
			c.logEvent("[HAZARD] SPILL DETECTED at " + h + " - Rerouting traffic!")
		}
	}

	for _, r := range c.Robots { 
		c.step(r, dt) 
		if r.Status == "waiting" {
			c.TotalDelay += dt
		}
	}
	c.breakDeadlocks()
}

// ─── helper: transit lock management ──────────────────────────────────────

func (c *Controller) lockNode(nodeID string, r *Robot) bool {
	// If it's a hazard zone, INSTANTLY BLOCK physical traversal
	if _, isHazard := c.Hazards[nodeID]; isHazard {
		return false
	}
	
	// Already hold it?
	for _, id := range c.transitLocks[nodeID] {
		if id == r.ID { return true }
	}
	cap := 1
	if c.Graph.Nodes[nodeID] != nil && c.Graph.Nodes[nodeID].IsChargingStation {
		if r.TargetNode == nodeID {
			// They are coming to charge: allow up to the station's full capacity
			cap = c.Graph.Nodes[nodeID].Capacity
			if cap <= 0 { cap = 1 }
		} else {
			// They are just passing through!
			// We only let them pass if the station is COMPLETELY EMPTY (0 robots).
			// If there's already 1 passing robot or 1 charging robot, they must yield.
			cap = 1 
		}
	}
	if len(c.transitLocks[nodeID]) >= cap { return false }
	c.transitLocks[nodeID] = append(c.transitLocks[nodeID], r.ID)
	return true
}

func (c *Controller) unlockNode(nodeID, robotID string) {
	holders := c.transitLocks[nodeID]
	for i, id := range holders {
		if id == robotID {
			c.transitLocks[nodeID] = append(holders[:i], holders[i+1:]...)
			break
		}
	}
	if len(c.transitLocks[nodeID]) == 0 { delete(c.transitLocks, nodeID) }
}

// ─── Movement step ─────────────────────────────────────────────────────────

func (c *Controller) step(r *Robot, dt float64) {
	if c.EStop { r.Status = StatusWaiting; return }

	// ── Energy Model ───────────────────────────────────────────────
	if r.Status != StatusCharging {
		r.BatteryLevel -= 2.0 * dt
		if r.BatteryLevel <= 3.0 { 
			r.BatteryLevel = 3.0
			r.Status = StatusWaiting
			r.Speed = 0
			c.deadTicks[r.ID]++
			if c.deadTicks[r.ID] > 100 { // Towing logic (approx 1.5s wait)
				r.BatteryLevel = 100.0
				c.deadTicks[r.ID] = 0
				r.Status = "idle"
				c.logEvent("[TOW] " + r.ID + " rescued from dead battery")
			}
			return
		}

		// Reroute if battery < 35% (higher safety margin for large grid)
		if r.BatteryLevel < 35.0 && len(r.Path) > 0 {
			targetNode := r.Path[len(r.Path)-1]
			if !c.Graph.Nodes[targetNode].IsChargingStation {
				r.Path = nil // Force recalculate to find charger
			}
		}
	}

	// ── A. Robot is moving on a lane ───────────────────────────────────────
	if r.CurrentLane != "" {
		lane := c.Graph.Lanes[r.CurrentLane]
		if lane == nil { r.CurrentLane = ""; return }

		// Base speed from metadata
		baseSpeed := lane.Metadata.MaxSpeed
		
		// 1. Safety requirements adaptation
		if lane.Metadata.LaneType == "human zone" {
			baseSpeed *= 0.4 // Drastically reduce speed in human zones for safety
		} else if lane.Metadata.SafetyLevel == "critical" {
			baseSpeed *= 0.6
		}

		// 2. Congestion & Lane Conditions adaptation
		// Speed drops up to 50% based on active lane congestion
		speed := baseSpeed * (1.0 - lane.CongestionScore*0.5)
		
		// Absolute minimum crawling speed
		if speed < 2 { speed = 2 }
		
		r.Progress += (speed * dt) / lane.Length
		r.Speed = speed
		r.Status = "moving"

		if r.Progress >= 1.0 {
			// Arrive at destination
			lane.leave(r.ID)
			lane.ReservedBy = ""

			// Unlock the SOURCE node (we are now fully through it)
			c.unlockNode(lane.Source, r.ID)

			r.CurrentNode = lane.Target
			r.CurrentLane = ""
			r.Progress = 0
			if len(r.Path) > 0 { r.Path = r.Path[1:] }
			c.stuckTicks[r.ID] = 0

			// We implicitly retain the lock on lane.Target because we locked it
			// before entering this lane, which mathematically prevents anyone else from approaching.
		}
		return
	}

	// ── B. Need a path ─────────────────────────────────────────────
	if len(r.Path) == 0 {
		if r.CurrentNode == r.TargetNode {
			node := c.Graph.Nodes[r.CurrentNode]
			
			// Are we at a charger and need juice?
			if node.IsChargingStation && r.BatteryLevel < 99.0 {
				if r.Status != StatusCharging {
					c.chargerOccupancy[r.CurrentNode]++
					c.logEvent("[CHARGE] " + r.ID + " docked at " + r.CurrentNode)
				}
				r.Status = StatusCharging
				r.BatteryLevel += 25.0 * dt
				if r.BatteryLevel > 100.0 { r.BatteryLevel = 100.0 }
				return
			}
			// Done charging — release slot
			if node.IsChargingStation && r.Status == StatusCharging {
				if c.chargerOccupancy[r.CurrentNode] > 0 { c.chargerOccupancy[r.CurrentNode]-- }
				c.logEvent("[CHARGE] " + r.ID + " done, leaving " + r.CurrentNode)
			}
			
			// Done intent
			if c.chargingIntent[r.CurrentNode] > 0 {
				c.chargingIntent[r.CurrentNode]--
			}

			// We successfully completed a regular delivery task
			if !node.IsChargingStation {
				c.GoalsReached++
				c.logEvent("[GOAL] " + r.ID + " delivered to " + r.CurrentNode)
			}
			
			// Find a new random target (avoid chargers unless low battery)
			keys := make([]string, 0, len(c.Graph.Nodes))
			for k, n := range c.Graph.Nodes { 
				if k != r.CurrentNode && !n.IsChargingStation { keys = append(keys, k) } 
			}
			if len(keys) > 0 { r.TargetNode = keys[rand.Intn(len(keys))] }
		}

		// Battery Check before pathing — pick nearest NON-FULL charger
		if r.BatteryLevel < 35.0 {
			var bestCharger string
			bestCost := math.MaxFloat64
			for id, node := range c.Graph.Nodes {
				if !node.IsChargingStation { continue }
				cap := node.Capacity
				if cap <= 0 { cap = 1 } // default single-port
				// Check combined occupancy + incoming intent
				if c.chargerOccupancy[id] + c.chargingIntent[id] >= cap { continue }
				dx := c.Graph.Nodes[r.CurrentNode].X - node.X
				dy := c.Graph.Nodes[r.CurrentNode].Y - node.Y
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < bestCost {
					bestCost = dist
					bestCharger = id
				}
			}
			if bestCharger != "" {
				r.TargetNode = bestCharger
				c.chargingIntent[bestCharger]++
				c.logEvent("[LOW BATT] " + r.ID + " routing to " + bestCharger)
			}
		}

		r.Path = c.aStar(r.CurrentNode, r.TargetNode, r.ID)
		if len(r.Path) == 0 { r.Status = StatusWaiting; c.stuckTicks[r.ID]++; return }
	}

	// ── C. Try entering the next lane ─────────────────────────────
	nextNode := r.Path[0]
	fwd := c.Graph.FindLane(r.CurrentNode, nextNode)
	if fwd == nil { r.Path = nil; return }

	// Guard 1 — lane capacity: only 1 robot per lane
	if len(fwd.Occupants) >= 1 {
		r.Status = "waiting"; c.stuckTicks[r.ID]++; return
	}

	// Guard 2 — head-on: reverse lane must be empty
	if rev := c.Graph.FindLane(nextNode, r.CurrentNode); rev != nil && len(rev.Occupants) > 0 {
		r.Status = "waiting"; c.stuckTicks[r.ID]++; return
	}

	// Guard 3 — lane reservation by another robot
	if fwd.ReservedBy != "" && fwd.ReservedBy != r.ID {
		r.Status = "waiting"; c.stuckTicks[r.ID]++; return
	}

	// Guard 4 — intersection (destination node) transit lock
	// Prevents two robots from being simultaneously near the same junction
	if !c.lockNode(nextNode, r) {
		r.Status = StatusWaiting; c.stuckTicks[r.ID]++; return
	}

	// All clear — enter the lane
	// Source node is already locked by CurrentNode lock, so we are safe
	fwd.ReservedBy = r.ID
	fwd.enter(r.ID)
	r.CurrentLane = fwd.ID
	r.Progress = 0
	r.Status = StatusGo
	c.stuckTicks[r.ID] = 0
}

// ─── Deadlock detection and resolution ────────────────────────────────────

func (c *Controller) breakDeadlocks() {
	// Build wait-for graph from transit locks
	waitFor := make(map[string]string)
	for _, r := range c.Robots {
		if r.Status != StatusWaiting || r.CurrentLane != "" || len(r.Path) == 0 {
			continue
		}
		nextNode := r.Path[0]
		if holders, held := c.transitLocks[nextNode]; held {
			for _, holder := range holders {
				if holder != r.ID {
					waitFor[r.ID] = holder
					break // Just wait for the first holder
				}
			}
		}
	}

	// DFS cycle detection
	inCycle := make(map[string]bool)
	for start := range waitFor {
		visited := make(map[string]bool)
		cur := start
		for {
			if visited[cur] {
				n := start
				for !inCycle[n] {
					inCycle[n] = true
					next, ok := waitFor[n]
					if !ok { break }
					n = next
				}
				inCycle[cur] = true
				break
			}
			visited[cur] = true
			if next, ok := waitFor[cur]; ok { cur = next } else { break }
		}
	}

	if len(inCycle) > 0 {
		var victim string
		minPriority := 999
		maxT := -1
		
		for id := range inCycle {
			r := c.Robots[id]
			if r == nil { continue }
			p := r.Priority
			
			// Always evict the lowest priority robot. If tied, evict the one waiting the longest.
			if p < minPriority {
				minPriority = p
				maxT = c.stuckTicks[id]
				victim = id
			} else if p == minPriority && c.stuckTicks[id] > maxT {
				maxT = c.stuckTicks[id]
				victim = id
			}
		}
		if r, ok := c.Robots[victim]; ok {
			for node := range c.transitLocks {
				if node != r.CurrentNode {
					c.unlockNode(node, r.ID)
				}
			}
			c.logEvent("[DEADLOCK] Resolved — rerouting " + victim)
			c.forceReroute(r)
			c.stuckTicks[victim] = 0
		}
		return
	}

	// Fallback: release and reroute robots stuck > 15 ticks
	for id, ticks := range c.stuckTicks {
		if r, ok := c.Robots[id]; ok {
			// Do not make high priority robots stubbornly wait 60 ticks! 
			// That creates massive unresolvable conga-lines. Everyone detours after 15 ticks.
			threshold := 15
			
			if ticks > threshold {
				for node := range c.transitLocks {
					if node != r.CurrentNode {
						c.unlockNode(node, r.ID)
					}
				}
				c.forceReroute(r)
				c.stuckTicks[id] = 0
			}
		}
	}
}

func (c *Controller) forceReroute(r *Robot) {
	// Release any future lane reservations held by this robot
	for _, l := range c.Graph.Lanes {
		if l.ReservedBy == r.ID && l.ID != r.CurrentLane {
			l.ReservedBy = ""
		}
	}
	
	// If heading to charger and forced to reroute, drop intent
	if len(r.Path) > 0 && c.Graph.Nodes[r.TargetNode].IsChargingStation {
		if c.chargingIntent[r.TargetNode] > 0 { c.chargingIntent[r.TargetNode]-- }
	}

	keys := make([]string, 0, len(c.Graph.Nodes))
	for k := range c.Graph.Nodes { if k != r.CurrentNode { keys = append(keys, k) } }
	if len(keys) == 0 { return }
	r.TargetNode = keys[rand.Intn(len(keys))]
	r.Path = c.aStar(r.CurrentNode, r.TargetNode, r.ID)
	r.Status = "idle"
}

// ─── A* ───────────────────────────────────────────────────────────────────

type aNode struct {
	id   string
	g, f float64
	path []string
}

func (c *Controller) aStar(start, goal, robotID string) []string {
	if start == goal { return []string{} }
	goalNode, ok := c.Graph.Nodes[goal]
	if !ok { return nil }

	open := []aNode{{id: start}}
	visited := map[string]bool{}
	for len(open) > 0 {
		best := 0
		for i := 1; i < len(open); i++ { if open[i].f < open[best].f { best = i } }
		curr := open[best]
		open = append(open[:best], open[best+1:]...)
		if curr.id == goal { return curr.path }
		if visited[curr.id] { continue }
		visited[curr.id] = true
		for _, lane := range c.Graph.Lanes {
			if lane.Source != curr.id || visited[lane.Target] { continue }
			
			// DO NOT path through toxic spills/hazards
			if _, isHazard := c.Hazards[lane.Target]; isHazard {
				continue
			}
			
			// Heavily penalize trying to path through a node already locked by someone else
			nodeCost := 0.0
			if holders, locked := c.transitLocks[lane.Target]; locked {
				for _, holder := range holders {
					if holder != robotID {
						nodeCost = 10000.0
						break
					}
				}
			}

			cost := lane.Length + lane.CongestionScore*lane.Length*2.0 + nodeCost
			tn := c.Graph.Nodes[lane.Target]
			dx := tn.X - goalNode.X; dy := tn.Y - goalNode.Y
			h := math.Sqrt(dx*dx + dy*dy)
			np := make([]string, len(curr.path)+1)
			copy(np, curr.path); np[len(curr.path)] = lane.Target
			g := curr.g + cost
			open = append(open, aNode{id: lane.Target, g: g, f: g + h, path: np})
		}
	}
	return nil
}
