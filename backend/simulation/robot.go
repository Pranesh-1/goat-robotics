package simulation

type RobotStatus string

const (
	StatusIdle     RobotStatus = "idle"
	StatusMoving   RobotStatus = "waiting" // kept for frontend colour compat
	StatusWaiting  RobotStatus = "waiting"
	StatusGo       RobotStatus = "moving"
	StatusCharging RobotStatus = "charging"
)

type Robot struct {
	ID          string      `json:"id"`
	CurrentNode string      `json:"currentNode"`
	CurrentLane string      `json:"currentLane"`
	TargetNode  string      `json:"targetNode"`
	Path        []string    `json:"path"`
	Speed        float64     `json:"speed"`
	Progress     float64     `json:"progress"`
	Status       RobotStatus `json:"status"`
	BatteryLevel float64     `json:"batteryLevel"` // 0.0 to 100.0
	Priority     int         `json:"priority"`     // 0=Low, 1=Normal, 2=High
}

func NewRobot(id, startNode, targetNode string, priority int) *Robot {
	return &Robot{
		ID:           id,
		CurrentNode:  startNode,
		TargetNode:   targetNode,
		Path:         []string{},
		Status:       StatusIdle,
		BatteryLevel: 100.0,
		Priority:     priority,
	}
}
