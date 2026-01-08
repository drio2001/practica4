// state.go
package main

import "context"

type WorkshopMode int

const (
	ModeInactive  WorkshopMode = iota // 0
	ModeOnlyA                         // 1
	ModeOnlyB                         // 2
	ModeOnlyC                         // 3
	ModePriorityA                     // 4
	ModePriorityB                     // 5
	ModePriorityC                     // 6
	ModeKeep                          // 7/8 (se mantiene)
	ModeClosed                        // 9
)

type StateManager struct {
	setCh chan int
	getCh chan chan int
	done  chan struct{}

	current int
}

func NewStateManager() *StateManager {
	sm := &StateManager{
		setCh: make(chan int, 32),
		getCh: make(chan chan int),
		done:  make(chan struct{}),
		// Estado inicial razonable: inactivo hasta que mutua envíe algo
		current: 0,
	}
	go sm.loop()
	return sm
}

func (sm *StateManager) loop() {
	for {
		select {
		case v := <-sm.setCh:
			// 7 y 8 mantienen el estado anterior. :contentReference[oaicite:7]{index=7}
			if v == 7 || v == 8 {
				continue
			}
			sm.current = v
		case ch := <-sm.getCh:
			ch <- sm.current
		case <-sm.done:
			return
		}
	}
}

func (sm *StateManager) Close() { close(sm.done) }

func (sm *StateManager) SetState(v int) {
	select {
	case sm.setCh <- v:
	default:
		// Si hay burst, no bloqueamos: nos quedamos con lo último que vaya entrando.
		sm.setCh <- v
	}
}

func (sm *StateManager) GetState(ctx context.Context) int {
	resp := make(chan int, 1)
	select {
	case sm.getCh <- resp:
	case <-ctx.Done():
		return 0
	}
	select {
	case v := <-resp:
		return v
	case <-ctx.Done():
		return 0
	}
}

// Decisión de "quién va primero" según estado actual (prioridad) o modo "solo X".
// Tabla de estados del enunciado: 0..9. :contentReference[oaicite:8]{index=8}
func PreferredOrder(state int) []Category {
	switch state {
	case 4: // prioridad A
		return []Category{CatA, CatB, CatC}
	case 5: // prioridad B
		return []Category{CatB, CatA, CatC}
	case 6: // prioridad C
		return []Category{CatC, CatA, CatB}
	default:
		// Orden base (A > B > C) por prioridad natural. :contentReference[oaicite:9]{index=9}
		return []Category{CatA, CatB, CatC}
	}
}

func AllowedOnly(state int, cat Category) bool {
	switch state {
	case 1:
		return cat == CatA
	case 2:
		return cat == CatB
	case 3:
		return cat == CatC
	default:
		return true
	}
}

func IsInactive(state int) bool { return state == 0 }
func IsClosed(state int) bool   { return state == 9 }
