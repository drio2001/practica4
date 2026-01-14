// workshop_test.go
package main

import (
	"context"
	"testing"
	"time"
)

func TestComparativas(t *testing.T) {
	type mix struct{ a, b, c int }
	mixes := []mix{
		{a: 10, b: 10, c: 10},
		{a: 20, b: 5, c: 5},
		{a: 5, b: 5, c: 20},
	}
	type cap struct{ plazas, mecanicos int }
	caps := []cap{
		{plazas: 6, mecanicos: 3},
		{plazas: 4, mecanicos: 4},
	}

	for mi, m := range mixes {
		for ci, cp := range caps {
			name := "mix" + itoa(mi+1) + "_cap" + itoa(ci+1)
			t.Run(name, func(t *testing.T) {
				cfg := DefaultConfig()
				cfg.NumA, cfg.NumB, cfg.NumC = m.a, m.b, m.c
				cfg.NumPlazas, cfg.NumMecanicos = cp.plazas, cp.mecanicos

				// Para tests: jitter bajo para que no se eternicen
				cfg.MaxJitterSeconds = 0

				ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
				defer cancel()

				sm := NewStateManager()
				defer sm.Close()

				// Estado 4 = prioridad A (pero sin filtrar categorías). :contentReference[oaicite:14]{index=14}
				sm.SetState(4)

				metrics := RunSimulation(ctx, cfg, sm)
				if metrics.TotalProcessed != (m.a + m.b + m.c) {
					t.Fatalf("procesados=%d esperado=%d", metrics.TotalProcessed, m.a+m.b+m.c)
				}

				t.Logf("plazas=%d mecanicos=%d A=%d B=%d C=%d total=%s",
					cp.plazas, cp.mecanicos, m.a, m.b, m.c, metrics.TotalDuration)
			})
		}
	}
}

func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, digits[n%10])
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
