import './style.css';

const canvas = document.getElementById('simCanvas');
const ctx = canvas.getContext('2d');

// UI Elements
const elRobots = document.getElementById('val-robots');
const elSpeed  = document.getElementById('val-speed');
const elThroughput = document.getElementById('val-throughput');
const elDelay = document.getElementById('val-delay');
const btnEStop = document.getElementById('btn-estop');
const elStatus = document.getElementById('val-status');

function loadPreset(name) {
  fetch('http://localhost:8080/api/preset?name=' + name)
    .then(r => {
      if (!r.ok) throw new Error('Preset not found: ' + name);
      return r.json();
    })
    .then(() => {
      document.querySelectorAll('.preset-btn').forEach(btn => btn.classList.remove('active'));
      document.getElementById('btn-' + name).classList.add('active');
      // Force transform recalculation for the new grid size
      transform = null;
      lastEventText = '';
    })
    .catch(err => console.error('Preset switch failed:', err));
}
window.loadPreset = loadPreset;

// State
let simulationState = { nodes: [], lanes: [], robots: [] };

// Canvas transform (computed once nodes arrive)
let transform = null; // { scaleX, scaleY, offsetX, offsetY }

// Resize canvas to fill its container
function resize() {
  const container = document.getElementById('canvas-container');
  const logPanel = document.getElementById('event-log');
  canvas.width  = container.clientWidth;
  canvas.height = container.clientHeight - (logPanel ? logPanel.offsetHeight : 0);
  transform = null;
}
window.addEventListener('resize', resize);
resize();

// ── Compute a centred, padded transform from the node bounding box ──
function computeTransform(nodes) {
  if (!nodes || nodes.length === 0) return null;

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  nodes.forEach(n => {
    if (n.x < minX) minX = n.x;
    if (n.y < minY) minY = n.y;
    if (n.x > maxX) maxX = n.x;
    if (n.y > maxY) maxY = n.y;
  });

  const PADDING = 80;
  const availW = canvas.width  - PADDING * 2;
  const availH = canvas.height - PADDING * 2;
  const gridW  = maxX - minX || 1;
  const gridH  = maxY - minY || 1;

  const scale = Math.min(availW / gridW, availH / gridH);

  // Centre in the available area
  const offsetX = PADDING + (availW - gridW * scale) / 2 - minX * scale;
  const offsetY = PADDING + (availH - gridH * scale) / 2 - minY * scale;

  return { scale, offsetX, offsetY };
}

// Map a node coordinate to canvas space
function tx(x) { return x * transform.scale + transform.offsetX; }
function ty(y) { return y * transform.scale + transform.offsetY; }

// ── WebSocket ──
function connect() {
  const ws = new WebSocket('ws://localhost:8080/ws');

  ws.onopen = () => {
    elStatus.textContent = 'Online';
    elStatus.className = 'metric-value text-green';
  };

  ws.onclose = () => {
    elStatus.textContent = 'Offline';
    elStatus.style.color = '#ef4444';
    setTimeout(connect, 2000);
  };

  ws.onmessage = (e) => {
    try {
      simulationState = JSON.parse(e.data);
      if (!transform && simulationState.nodes && simulationState.nodes.length > 0) {
        transform = computeTransform(simulationState.nodes);
      }
      updateMetrics();
      if (simulationState.eventLog) updateEventLog(simulationState.eventLog);
    } catch (err) { console.error(err); }
  };
}

function updateMetrics() {
  const robots = simulationState.robots;
  if (!robots) return;
  elRobots.textContent = robots.length;
  const avgSpeed = robots.length
    ? (robots.reduce((s, r) => s + r.speed, 0) / robots.length).toFixed(1)
    : '0.0';
  elSpeed.textContent = avgSpeed;

  if (simulationState.metrics) {
    elThroughput.textContent = simulationState.metrics.throughput.toFixed(1);
    elDelay.textContent = simulationState.metrics.delay.toFixed(1);
  }
}

let lastEventText = '';
function updateEventLog(events) {
  if (!events || events.length === 0) return;
  // Compare the newest event text to detect rotation in the circular buffer
  const newest = events[events.length - 1];
  if (newest === lastEventText) return;
  lastEventText = newest;

  const body = document.getElementById('event-log-body');
  const counter = document.getElementById('event-log-count');
  if (!body) return;

  counter.textContent = events.length + ' events';

  // Newest events first (prepend at top)
  body.innerHTML = '';
  const reversed = [...events].reverse();
  reversed.forEach(msg => {
    const div = document.createElement('div');
    div.classList.add('log-entry');
    if (msg.includes('[GOAL]'))          div.classList.add('goal');
    else if (msg.includes('[CHARGE]'))   div.classList.add('charge');
    else if (msg.includes('[LOW BATT]')) div.classList.add('lowbat');
    else if (msg.includes('[DEADLOCK]')) div.classList.add('deadlock');
    else if (msg.includes('[ESTOP]'))    div.classList.add('estop');
    else if (msg.includes('[TOW]'))      div.classList.add('tow');
    else                                 div.classList.add('sys');
    div.textContent = msg;
    body.appendChild(div);
  });
  body.scrollTop = 0; // Keep newest in view
}

// ── Event Listeners ──
btnEStop.addEventListener('click', async () => {
  try {
    const res = await fetch('http://localhost:8080/api/estop', { method: 'POST' });
    const data = await res.json();
    if (data.estop) {
      btnEStop.classList.add('active');
      btnEStop.textContent = 'RESUME ALL';
    } else {
      btnEStop.classList.remove('active');
      btnEStop.textContent = 'EMERGENCY STOP';
    }
  } catch (err) {
    console.error('E-Stop Error:', err);
  }
});

// ── Draw Loop ──
function draw() {
  const { nodes, lanes, robots } = simulationState;

  if (canvas.width === 0 || canvas.height === 0) {
    const c = document.getElementById('canvas-container');
    if (c.clientWidth > 0) {
      canvas.width = c.clientWidth;
      canvas.height = c.clientHeight;
      transform = computeTransform(nodes); // Force refresh
    }
  }

  ctx.clearRect(0, 0, canvas.width, canvas.height);

  // DEBUG: Draw a massive stroke to prove the canvas is not completely hidden
  ctx.strokeStyle = '#f43f5e';
  ctx.lineWidth = 10;
  ctx.strokeRect(0, 0, canvas.width, canvas.height);

  if (!nodes || nodes.length === 0 || !transform) {
    requestAnimationFrame(draw);
    return;
  }

  // Build lookup maps for O(1) access
  const nodeMap = {};
  (nodes || []).forEach(n => nodeMap[n.id] = n);
  const laneMap = {};
  (lanes || []).forEach(l => laneMap[l.id] = l);

  // Determine intended lanes natively on the client to avoid backend locking ghosts
  const intendedLanes = new Set();
  (robots || []).forEach(robot => {
    if (!robot.currentLane && robot.path && robot.path.length > 0) {
      intendedLanes.add(robot.currentNode + '-' + robot.path[0]);
    }
  });

  // Build visual undirected edge map to prevent violent overlaps
  const drawEdges = {};
  (lanes || []).forEach(lane => {
    const src = nodeMap[lane.source];
    const tgt = nodeMap[lane.target];
    if (!src || !tgt) return;

    // Create undirected key
    const idA = src.id < tgt.id ? src.id : tgt.id;
    const idB = src.id < tgt.id ? tgt.id : src.id;
    const edgeId = idA + '-' + idB;

    const isOccupied = lane.occupants && lane.occupants.length > 0;
    
    // It's formally reserved if the backend claims it OR if a robot is idling at a node intending to enter it
    const intentionallyReserved = lane.reservedBy || intendedLanes.has(lane.source + '-' + lane.target);
    const isReserved = !isOccupied && intentionallyReserved;

    let priority = 0; // Free
    if (isReserved) priority = 1;
    if (isOccupied) priority = 2;

    if (!drawEdges[edgeId] || priority > drawEdges[edgeId].priority) {
      drawEdges[edgeId] = { src, tgt, priority };
    }
  });

  // ── Draw Clean Edges ──
  Object.values(drawEdges).forEach(edge => {
    ctx.shadowBlur = 0;
    ctx.setLineDash([]);

    let color, lw;
    if (edge.priority === 2) {
      // Occupied — Sharp Neon Purple
      color = '#bd00ff';
      ctx.shadowColor = '#bd00ff';
      ctx.shadowBlur = 15;
      lw = 4.5;
    } else if (edge.priority === 1) {
      // Reserved — Sharp Electric Cyan
      color = '#00f3ff'; 
      ctx.shadowColor = '#00f3ff';
      ctx.shadowBlur = 8;
      ctx.setLineDash([5, 8]);
      lw = 2.5;
    } else {
      // Free — Deep Stealth
      color = 'rgba(255, 255, 255, 0.05)'; 
      lw = 1.5;
    }

    ctx.strokeStyle = color;
    ctx.lineWidth = lw;
    ctx.beginPath();
    ctx.moveTo(tx(edge.src.x), ty(edge.src.y));
    ctx.lineTo(tx(edge.tgt.x), ty(edge.tgt.y));
    ctx.stroke();

    ctx.shadowBlur = 0;
    ctx.setLineDash([]);
  });

  // ── Draw Nodes ──
  nodes.forEach(n => {
    ctx.fillStyle = n.isChargingStation ? '#166534' : '#0f172a'; // Green for chargers
    ctx.strokeStyle = n.isChargingStation ? '#22c55e' : '#1e293b'; 
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(tx(n.x), ty(n.y), n.isChargingStation ? 7 : 5, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();

    if (n.isChargingStation) {
      ctx.fillStyle = '#4ade80';
      ctx.font = '8px Arial';
      ctx.fillText('⚡', tx(n.x) - 4, ty(n.y) + 3);
    }
  });

  // ── Draw Hazards ──
  const hazards = simulationState.hazards || [];
  hazards.forEach(hazardId => {
    const n = nodeMap[hazardId];
    if (!n) return;
    
    // Flashing red zone
    ctx.beginPath();
    ctx.arc(tx(n.x), ty(n.y), 15, 0, Math.PI * 2);
    ctx.fillStyle = (Date.now() % 500 < 250) ? 'rgba(239, 68, 68, 0.4)' : 'transparent';
    ctx.fill();
    ctx.strokeStyle = '#ef4444';
    ctx.lineWidth = 2;
    ctx.stroke();

    // Icon
    ctx.fillStyle = '#fca5a5';
    ctx.font = '14px Arial';
    ctx.fillText('⚠️', tx(n.x) - 10, ty(n.y) + 5);
  });

  // ── Draw Robots ──
  const nodeCount = {};
  const nodeIndex = {};
  (robots || []).forEach(robot => {
    if (!robot.currentLane) {
      nodeCount[robot.currentNode] = (nodeCount[robot.currentNode] || 0) + 1;
    }
  });

  (robots || []).forEach(robot => {
    const node = nodeMap[robot.currentNode];
    if (!node) return;

    let rx = node.x, ry = node.y;

    if (robot.currentLane) {
      const lane = laneMap[robot.currentLane];
      if (lane) {
        const tgtNode = nodeMap[lane.target];
        if (tgtNode) {
          const p = Math.min(Math.max(robot.progress, 0), 1);
          rx = node.x + (tgtNode.x - node.x) * p;
          ry = node.y + (tgtNode.y - node.y) * p;
        }
      }
    } else {
      const count = nodeCount[robot.currentNode] || 1;
      if (count > 1) {
        nodeIndex[robot.currentNode] = (nodeIndex[robot.currentNode] || 0);
        const idx = nodeIndex[robot.currentNode]++;
        const angle = (idx / count) * Math.PI * 2;
        rx += Math.cos(angle) * 24; // Increased radius to prevent overlap
        ry += Math.sin(angle) * 24;
        
        // Draw Cyberpunk Tether to the shared node
        ctx.strokeStyle = robot.status === 'charging' ? '#3b82f6' : '#64748b';
        ctx.lineWidth = 1.5;
        ctx.setLineDash([2, 4]);
        ctx.beginPath();
        ctx.moveTo(tx(node.x), ty(node.y));
        ctx.lineTo(tx(rx), ty(ry));
        ctx.stroke();
        ctx.setLineDash([]);
      }
    }

    // Robot color by status
    let color = '#94a3b8';   // idle — slate
    let glow = '#94a3b8';
    if (robot.status === 'moving') {
      color = '#00ff89';     // Sharp Emerald
      glow = '#00ff89';
    } else if (robot.status === 'charging') {
      color = '#3b82f6';     // Electric Blue for charging
      glow = '#3b82f6';
    } else if (robot.status === 'emergency') {
      color = '#ff003c';     // Sharp Ruby
      glow = '#ff003c';
    } else if (robot.status === 'waiting') {
      color = '#ff2d55';     // Sharp Rose
      glow = '#ff2d55';
    }

    // Overrides for priority rendering
    let radius = 8;
    if (robot.priority === 2) {
      ctx.shadowBlur = 25; // Massive glowing aura
      ctx.shadowColor = '#eab308'; // Golden aura for VIP
      color = '#fde047'; // Bright yellow core
      radius = 10; // Slightly larger
    } else if (robot.priority === 0) {
      radius = 6; // Smaller for low priority
      ctx.shadowBlur = robot.status !== 'idle' ? 8 : 0;
    } else {
      ctx.shadowBlur = robot.status !== 'idle' ? 15 : 0;
      ctx.shadowColor = glow;
    }

    // Body
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(tx(rx), ty(ry), radius, 0, Math.PI * 2);
    ctx.fill();

    // Sharp White Outline for contrast
    ctx.strokeStyle = (robot.priority === 2) ? '#ffffff' : 'white';
    ctx.lineWidth = (robot.priority === 2) ? 2.5 : 1.5;
    ctx.stroke();

    // Battery Bar
    if (robot.batteryLevel !== undefined) {
      const batW = 16;
      const batH = 3;
      const batX = tx(rx) - batW/2;
      const batY = ty(ry) + 12; // below robot
      
      // Background
      ctx.fillStyle = 'rgba(0,0,0,0.8)';
      ctx.fillRect(batX, batY, batW, batH);
      
      // Fill
      const fillW = Math.max(0, (robot.batteryLevel / 100) * batW);
      ctx.fillStyle = robot.batteryLevel > 20 ? '#10b981' : '#ef4444'; // Green or Red
      ctx.fillRect(batX, batY, fillW, batH);
    }

    ctx.shadowBlur = 0;
    ctx.fillStyle = '#fff';
    ctx.font = 'bold 10px Inter';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(robot.id, tx(rx), ty(ry) - 20);
  });

  requestAnimationFrame(draw);
}

connect();
requestAnimationFrame(draw);
