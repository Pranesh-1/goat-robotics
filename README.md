# GOAT Robotics - Industrial Fleet Management System

GOAT Robotics is an enterprise-grade autonomous grid simulation and fleet management hypervisor. Engineered in Go and React, the platform orchestrates complex Swarm AI pathing, physics-compliant intersection constraints, and high-density industrial telemetry in real-time.

## Architecture

The system utilizes a decoupled architecture to guarantee high-performance 60Hz physics and rendering isolation:

- **Hypervisor (Backend)**: Built in Go. Operates a stateful grid engine utilizing mathematical A* routing algorithms, highly-penalized transit locks, cyclic deadlock detection via Depth-First Search (DFS), and dynamic environmental hazard tracking. Payload snapshots are pushed out natively via WebSockets.
- **Frontend Dashboard**: Built in Vite and React. Captures deep telemetric data payloads and renders an aesthetic, frame-perfect canvas representing traffic flow, congestion scores, battery margins, and E-Stop protocols.

## Core Capabilities

- **Intelligent Routing**: Dynamic A* pathfinding strictly penalizes congested lanes and actively avoids unpredictable grid hazards (e.g., chemical spills) by immediately rerouting highly active units in wide structural arcs.
- **Priority Tiering**: Instantiates complex hierarchical right-of-way routing. High Priority (VIP) dispatch units aggressively hold locks and mathematically repel lower-tier robots from gridlock centers, enforcing fluid bypass lanes.
- **Physics-Compliant Transit**: Enforces intersection constraints ensuring pass-through lanes collapse appropriately when a robotic unit engages a docking action at a multi-tier charging port limit.
- **Deadlock Resolution**: Employs an aggressive cyclic deadlock tracker. When a locked loop occurs, the supervisor maps the wait-for graph, identifies the lowest-tier participant in the knot, and forcefully invalidates their pathing lock to rapidly evaporate the traffic jam.
- **Energy Lifecycle**: Simulates finite lithium-ion discharge. Robots monitor real-time capacity and reserve emergency charging intent bounds prior to draining below threshold limitations. Dead robots activate towing protocols.
- **Reporting Engine**: Synthesizes live 60-second slice analytics into PDF telemetry reports capturing active hazard states, unit delays, and VIP deployment margins.

## Execution Requirements

- Go 1.21 or higher
- Node.js 18 or higher

## Deployment Instructions

### Initiating the Go Telemetry Engine

1. Navigate to the backend directory.
2. Ensure network port 8080 is available.
3. Establish the execution environment:

```bash
cd backend
go run main.go report_generator.go
```

The Go engine will immediately mount the websocket framework and output operational lifecycle events.

### Initiating the React Dashboard

1. Navigate to the frontend directory.
2. Compile and mount the Vite instance:

```bash
cd frontend
npm install
npm run dev
```

Navigate to the localhost port provided by Vite (typically 5173). The dashboard will automatically connect to the backend socket layer and begin streaming the autonomous swarm logic.

## Configuration Profiles

The hypervisor supports hot-swapping configuration profiles in real-time. Modify `backend/config.yaml` to drastically alter the density grid width, length, lane velocity, and maximum active dispatch capacity. Alterations directly impact deadlock frequency and processor overhead calculations.
