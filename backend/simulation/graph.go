package simulation

import "math"

// No per-struct mutexes — the Controller owns one global lock for everything.

type Node struct {
	ID                string  `json:"id"`
	X                 float64 `json:"x"`
	Y                 float64 `json:"y"`
	IsChargingStation bool    `json:"isChargingStation"`
	Capacity          int     `json:"capacity"` // max robots that can use this simultaneously
}

type LaneMetadata struct {
	MaxSpeed    float64 `json:"maxSpeed"`
	SafetyLevel string  `json:"safetyLevel"`
	LaneType    string  `json:"laneType"`
}

type Lane struct {
	ID                   string       `json:"id"`
	Source               string       `json:"source"`
	Target               string       `json:"target"`
	Metadata             LaneMetadata `json:"metadata"`
	Length               float64      `json:"length"`
	CongestionScore      float64      `json:"congestionScore"`
	HistoricalUsageCount int          `json:"historicalUsageCount"`
	Occupants            []string     `json:"occupants"`
	ReservedBy           string       `json:"reservedBy"`
}

type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Lanes map[string]*Lane `json:"lanes"`
}

func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		Lanes: make(map[string]*Lane),
	}
}

func (g *Graph) AddNode(id string, x, y float64) {
	g.Nodes[id] = &Node{ID: id, X: x, Y: y}
}

func (g *Graph) AddLane(id, src, tgt string, maxSpeed float64, safetyLevel, laneType string) {
	srcNode := g.Nodes[src]
	tgtNode := g.Nodes[tgt]
	length := 100.0
	if srcNode != nil && tgtNode != nil {
		dx := tgtNode.X - srcNode.X
		dy := tgtNode.Y - srcNode.Y
		length = math.Sqrt(dx*dx + dy*dy)
	}
	g.Lanes[id] = &Lane{
		ID:        id,
		Source:    src,
		Target:    tgt,
		Metadata:  LaneMetadata{MaxSpeed: maxSpeed, SafetyLevel: safetyLevel, LaneType: laneType},
		Length:    length,
		Occupants: []string{},
	}
}

// All helpers below assume the caller (Controller) already holds its mutex.

func (g *Graph) FindLane(src, tgt string) *Lane {
	for _, l := range g.Lanes {
		if l.Source == src && l.Target == tgt {
			return l
		}
	}
	return nil
}

func (l *Lane) enter(robotID string) {
	l.HistoricalUsageCount++
	l.Occupants = append(l.Occupants, robotID)
	l.recalcCongestion()
}

func (l *Lane) leave(robotID string) {
	for i, id := range l.Occupants {
		if id == robotID {
			l.Occupants = append(l.Occupants[:i], l.Occupants[i+1:]...)
			break
		}
	}
	l.recalcCongestion()
}

func (l *Lane) recalcCongestion() {
	cap := 1.0 // default capacity for 'narrow' and 'intersection'

	switch l.Metadata.LaneType {
	case "normal":
		cap = 2.0 // Standard lane can allow tight following
	case "human zone":
		cap = 1.0 // High restriction
	}

	score := float64(len(l.Occupants)) / cap
	if score > 1.0 {
		score = 1.0
	}
	l.CongestionScore = score
}
