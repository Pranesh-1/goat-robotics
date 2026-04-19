package main

import (
	"bytes"
	"fmt"
	"time"

	"goat-backend/simulation"

	"github.com/jung-kurt/gofpdf"
)

func GeneratePDFReport(ctrl *simulation.Controller, presetName string) ([]byte, error) {
	metrics := ctrl.GetMetrics()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	
	// Enterprise Header
	pdf.SetFillColor(15, 23, 42)
	pdf.Rect(0, 0, 210, 40, "F")
	
	pdf.SetFont("Arial", "B", 24)
	pdf.SetTextColor(0, 243, 255) // Cyan
	pdf.SetY(12)
	pdf.CellFormat(190, 15, "GOAT Robotics", "0", 1, "C", false, 0, "")
	
	pdf.SetFont("Arial", "", 12)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(190, 6, "Mission-Critical Fleet Telemetry Report", "0", 1, "C", false, 0, "")
	pdf.Ln(20)

	// Reset text color for body
	pdf.SetTextColor(40, 40, 40)

	// Meta Informational Block
	pdf.SetFont("Arial", "B", 12)
	if presetName == "" { presetName = "Custom Engine" }
	pdf.CellFormat(95, 8, fmt.Sprintf("Configuration Profile: %s", presetName), "B", 0, "L", false, 0, "")
	pdf.CellFormat(95, 8, fmt.Sprintf("Timestamp: %s", time.Now().Format("2006-01-02 15:04 MST")), "B", 1, "R", false, 0, "")
	pdf.Ln(10)

	// Executive Summary
	pdf.SetFont("Arial", "", 11)
	summary := fmt.Sprintf("The GOAT Robotics orchestration engine is currently managing a unified swarm of %d industrial autonomous vehicles. This report details the real-time operational efficiency, bottleneck latency, and VIP priority load-balancing metrics of the active layout.", metrics.TotalRobots)
	pdf.MultiCell(190, 6, summary, "0", "L", false)
	pdf.Ln(10)

	// KPIs Data Table
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(15, 23, 42)
	pdf.Cell(190, 10, "System Performance KPIs")
	pdf.Ln(12)

	// Table Header
	pdf.SetFillColor(241, 245, 249) // Slate-100
	pdf.SetFont("Arial", "B", 11)
	pdf.CellFormat(120, 10, "Metric Category", "1", 0, "L", true, 0, "")
	pdf.CellFormat(70, 10, "Live Value", "1", 1, "C", true, 0, "")

	// Table Data Drawer Function
	drawRow := func(label, val string, isHighlight bool) {
		pdf.SetFont("Arial", "", 11)
		if isHighlight { pdf.SetTextColor(0, 153, 51) } else { pdf.SetTextColor(51, 65, 85) }
		pdf.CellFormat(120, 10, " " + label, "1", 0, "L", false, 0, "")
		
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(70, 10, val, "1", 1, "C", false, 0, "")
		pdf.SetTextColor(51, 65, 85) // Reset
	}

	drawRow("Total Active Fleet Vehicles", fmt.Sprintf("%d units", metrics.TotalRobots), false)
	drawRow("High-Priority (VIP) Interceptors", fmt.Sprintf("%d units", metrics.VIPCount), false)
	isSafe := metrics.ActiveHazards == 0
	drawRow("Unresolved Grid Hazards / Chemical Spills", fmt.Sprintf("%d active", metrics.ActiveHazards), isSafe)
	drawRow("Cumulative Lifecycle Deliveries", fmt.Sprintf("%d goals", metrics.GoalsReached), true)
	drawRow("Real-Time Swarm Throughput", fmt.Sprintf("%.1f goals/min", metrics.Throughput), false)
	drawRow("Cumulative Network Traffic Delay", fmt.Sprintf("%.2f seconds", metrics.TotalDelay), false)

	pdf.Ln(25)

	// Footer
	pdf.SetY(-30)
	pdf.SetFont("Arial", "I", 9)
	pdf.SetTextColor(148, 163, 184)
	pdf.CellFormat(190, 10, "Generated autonomously by the GOAT Fleet Management Hypervisor.", "0", 1, "C", false, 0, "")

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
