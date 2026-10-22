package domain

import "testing"

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Status
	}{
		{"english mixed case", "OnGoing", StatusOngoing},
		{"english hyphenated", "On-going", StatusOngoing},
		{"portuguese with spaces", "Em Andamento", StatusOngoing},
		{"spanish", "En curso", StatusOngoing},
		{"completed", "Completed", StatusCompleted},
		{"portuguese accent", "Concluído", StatusCompleted},
		{"finalizado", "Finalizado", StatusCompleted},
		{"hiatus", "Hiatus", StatusHiatus},
		{"on hold", "On Hold", StatusHiatus},
		{"hiato", "Em hiato", StatusHiatus},
		{"dropped", "Dropped", StatusCancelled},
		{"cancelado", "Cancelado", StatusCancelled},
		{"surrounding noise", "Status: OnGoing\n", StatusOngoing},
		{"empty", "", StatusUnknown},
		{"only symbols", " - ", StatusUnknown},
		{"unrecognized", "Season 2 soon", StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeStatus(tt.raw); got != tt.want {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
