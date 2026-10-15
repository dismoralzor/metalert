package main

import (
	"fmt"
	"testing"

	"github.com/dismoralzor/metalert/internal/model"
)

// Проверяем, что имена CPUutilizationN генерируются по фактическому числу
// значений от cpu.Percent(..., true) - то есть по числу логических CPU,
// а не захардкожены. Реальный gopsutil тут не нужен: buildCPUMetrics принимает
// уже готовый []float64, ровно так же, как получил бы его от cpu.Percent.
func TestBuildCPUMetrics_NamesMatchCoreCount(t *testing.T) {
	tests := []struct {
		name        string
		percentages []float64
	}{
		{"single core", []float64{42.0}},
		{"quad core", []float64{1.1, 2.2, 3.3, 4.4}},
		{"no cores reported", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCPUMetrics(tt.percentages)

			if len(got) != len(tt.percentages) {
				t.Fatalf("buildCPUMetrics() returned %d metrics, want %d", len(got), len(tt.percentages))
			}

			for i, m := range got {
				wantID := fmt.Sprintf("CPUutilization%d", i+1)
				if m.ID != wantID {
					t.Errorf("metric[%d].ID = %q, want %q", i, m.ID, wantID)
				}
				if m.MType != models.Gauge {
					t.Errorf("metric[%d].MType = %q, want %q", i, m.MType, models.Gauge)
				}
				if m.Value == nil || *m.Value != tt.percentages[i] {
					t.Errorf("metric[%d].Value = %v, want %v", i, m.Value, tt.percentages[i])
				}
			}
		})
	}
}
