// workshop.go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"
)

// Log requerido por enunciado: entrada/salida de cada fase.
// "Tiempo {Tiempo_Ejecución_Programa} Coche {N} Incidencia {Tipo} Fase {Fase_Actual} Estado {Estado_Fase}"
// :contentReference[oaicite:10]{index=10}
func logPhase(start time.Time, car Car, phase Phase, state string) {
	elapsed := time.Since(start).Truncate(time.Millisecond)
	fmt.Printf("Tiempo %s Coche %d Incidencia %s Fase %d Estado %s\n",
		elapsed, car.ID, car.Category.String(), int(phase), state)
}

// Un "semaphore" con canales (buffer) para no usar sync.Mutex.
type chanSem struct{ ch chan struct{} }

func newChanSem(n int) chanSem {
	s := chanSem{ch: make(chan struct{}, n)}
	for i := 0; i < n; i++ {
		s.ch <- struct{}{}
	}
	return s
}
func (s chanSem) Acquire(ctx context.Context) bool {
	select {
	case <-s.ch:
		return true
	case <-ctx.Done():
		return false
	}
}
func (s chanSem) Release() { s.ch <- struct{}{} }

type stageInput struct {
	A <-chan Car
	B <-chan Car
	C <-chan Car
}

// pickNext elige el próximo coche respetando:
// - Inactivo (0): no mueve coches.
// - Cerrado (9): no mueve coches.
// - Only A/B/C: bloquea categorías no permitidas.
// - Prioridad A/B/C: cambia orden de atención.
// (aplicado por estamento/fase: el dispatcher se usa en cada fase). :contentReference[oaicite:11]{index=11}
func pickNext(ctx context.Context, sm *StateManager, in stageInput) (Car, bool) {
	// Para evitar "busy loop", usamos un pequeño tick cuando no hay nada.
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		state := sm.GetState(ctx)
		if IsInactive(state) || IsClosed(state) {
			select {
			case <-tick.C:
				continue
			case <-ctx.Done():
				return Car{}, false
			}
		}

		order := PreferredOrder(state)
		// Intento no-bloqueante en el orden preferido
		for _, cat := range order {
			if !AllowedOnly(state, cat) {
				continue
			}
			switch cat {
			case CatA:
				select {
				case c, ok := <-in.A:
					if !ok {
						break
					}
					return c, true
				default:
				}
			case CatB:
				select {
				case c, ok := <-in.B:
					if !ok {
						break
					}
					return c, true
				default:
				}
			case CatC:
				select {
				case c, ok := <-in.C:
					if !ok {
						break
					}
					return c, true
				default:
				}
			}
		}

		// Si no se pudo coger nada sin bloquear, hacemos un select bloqueante pero respetando orden:
		// (solo con los canales permitidos)
		cases := make([]reflectCase, 0, 3)
		for _, cat := range order {
			if !AllowedOnly(state, cat) {
				continue
			}
			switch cat {
			case CatA:
				cases = append(cases, rcRecv(in.A, CatA))
			case CatB:
				cases = append(cases, rcRecv(in.B, CatB))
			case CatC:
				cases = append(cases, rcRecv(in.C, CatC))
			}
		}
		if len(cases) == 0 {
			select {
			case <-tick.C:
				continue
			case <-ctx.Done():
				return Car{}, false
			}
		}

		idx, val, ok := reflectSelect(ctx, cases, tick.C)
		if !ok {
			// canal cerrado o ctx cancelado
			if ctx.Err() != nil {
				return Car{}, false
			}
			_ = idx
			continue
		}
		_ = val // ya es Car
		return val, true
	}
}

// --- Pequeña capa para simular "reflect.Select" sin importar reflect en toda la práctica ---
// (manteniendo el fichero compacto y controlado).

type reflectCase struct {
	cat  Category
	recv <-chan Car
}

func rcRecv(ch <-chan Car, cat Category) reflectCase {
	return reflectCase{cat: cat, recv: ch}
}

func reflectSelect(ctx context.Context, cases []reflectCase, tick <-chan time.Time) (int, Car, bool) {
	// Implementación manual: select con 3 canales máximo (A,B,C) + tick + ctx.
	// Dado que como mucho serán 3, podemos escribirlo explícito.
	var a, b, c <-chan Car
	var haveA, haveB, haveC bool
	for _, cs := range cases {
		switch cs.cat {
		case CatA:
			a, haveA = cs.recv, true
		case CatB:
			b, haveB = cs.recv, true
		case CatC:
			c, haveC = cs.recv, true
		}
	}

	select {
	case v, ok := <-a:
		if haveA {
			return 0, v, ok
		}
	case v, ok := <-b:
		if haveB {
			return 1, v, ok
		}
	case v, ok := <-c:
		if haveC {
			return 2, v, ok
		}
	case <-tick:
		return -1, Car{}, false
	case <-ctx.Done():
		return -1, Car{}, false
	}
	return -1, Car{}, false
}

// RunSimulation crea N coches por categoría (orden aleatorio), los pasa por 4 fases, y respeta:
// - plazas (token global hasta que el coche sale del taller)
// - mecánicos (token en fase 2)
// - cleaners/reviewers (tokens fase 3/4)
// - colas máximas (buffers por fase)
// - prioridad/solo-categoría según estado mutua
func RunSimulation(ctx context.Context, cfg Config, sm *StateManager) Metrics {
	start := time.Now()
	rng := cfg.Rand()

	metrics := NewMetrics()

	// Recursos
	plazas := newChanSem(cfg.NumPlazas)
	mecanicos := newChanSem(cfg.NumMecanicos)
	cleaners := newChanSem(cfg.NumCleaners)
	reviewers := newChanSem(cfg.NumReviewers)

	// Colas por fase y categoría (buffer = max wait)
	// Fase 1: llegan coches desde "la calle" (a la cola de espera para entrar)
	p1A := make(chan Car, cfg.MaxWaitP1)
	p1B := make(chan Car, cfg.MaxWaitP1)
	p1C := make(chan Car, cfg.MaxWaitP1)

	// Fase 2: cola hacia mecánicos
	p2A := make(chan Car, cfg.MaxWaitP2)
	p2B := make(chan Car, cfg.MaxWaitP2)
	p2C := make(chan Car, cfg.MaxWaitP2)

	// Fase 3: cola limpieza
	p3A := make(chan Car, cfg.MaxWaitP3)
	p3B := make(chan Car, cfg.MaxWaitP3)
	p3C := make(chan Car, cfg.MaxWaitP3)

	// Fase 4: cola entrega
	p4A := make(chan Car, cfg.MaxWaitP4)
	p4B := make(chan Car, cfg.MaxWaitP4)
	p4C := make(chan Car, cfg.MaxWaitP4)

	done := make(chan Car, cfg.NumA+cfg.NumB+cfg.NumC)

	// Generación aleatoria por categoría (orden aleatorio dentro de cada categoría) :contentReference[oaicite:12]{index=12}
	go generateCars(ctx, rng, CatA, cfg.NumA, p1A)
	go generateCars(ctx, rng, CatB, cfg.NumB, p1B)
	go generateCars(ctx, rng, CatC, cfg.NumC, p1C)

	// --- Fase 1: esperar plaza + documentación ---
	go func() {
		dispatchStage(ctx, sm, stageInput{A: p1A, B: p1B, C: p1C}, func(car Car) bool {
			// Si taller cerrado, no aceptamos nuevos (pero dejamos drenar lo que ya esté dentro)
			if IsClosed(sm.GetState(ctx)) {
				return false
			}
			// plaza libre
			if !plazas.Acquire(ctx) {
				return false
			}
			logPhase(start, car, Phase1Docs, "ENTRA")
			sleepWithJitter(ctx, rng, BaseSeconds(car.Category), cfg.MaxJitterSeconds)
			logPhase(start, car, Phase1Docs, "SALE")

			// Pasa a fase 2 (manteniendo “plaza” ocupada hasta que salga del taller)
			sendByCategory(ctx, car, p2A, p2B, p2C)
			return true
		})
	}()

	// --- Fase 2: esperar mecánico + reparación ---
	go func() {
		dispatchStage(ctx, sm, stageInput{A: p2A, B: p2B, C: p2C}, func(car Car) bool {
			if !mecanicos.Acquire(ctx) {
				return false
			}
			logPhase(start, car, Phase2Mechanic, "ENTRA")
			sleepWithJitter(ctx, rng, BaseSeconds(car.Category), cfg.MaxJitterSeconds)
			logPhase(start, car, Phase2Mechanic, "SALE")
			mecanicos.Release()

			sendByCategory(ctx, car, p3A, p3B, p3C)
			return true
		})
	}()

	// --- Fase 3: limpieza ---
	go func() {
		dispatchStage(ctx, sm, stageInput{A: p3A, B: p3B, C: p3C}, func(car Car) bool {
			if !cleaners.Acquire(ctx) {
				return false
			}
			logPhase(start, car, Phase3Clean, "ENTRA")
			sleepWithJitter(ctx, rng, BaseSeconds(car.Category), cfg.MaxJitterSeconds)
			logPhase(start, car, Phase3Clean, "SALE")
			cleaners.Release()

			sendByCategory(ctx, car, p4A, p4B, p4C)
			return true
		})
	}()

	// --- Fase 4: entrega / revisión final + salida del taller ---
	go func() {
		dispatchStage(ctx, sm, stageInput{A: p4A, B: p4B, C: p4C}, func(car Car) bool {
			if !reviewers.Acquire(ctx) {
				return false
			}
			logPhase(start, car, Phase4Delivery, "ENTRA")
			sleepWithJitter(ctx, rng, BaseSeconds(car.Category), cfg.MaxJitterSeconds)
			logPhase(start, car, Phase4Delivery, "SALE")
			reviewers.Release()

			// sale del taller -> libera plaza
			plazas.Release()

			done <- car
			return true
		})
	}()

	total := cfg.NumA + cfg.NumB + cfg.NumC
	for i := 0; i < total; i++ {
		select {
		case car := <-done:
			metrics.TotalProcessed++
			metrics.ByCategory[car.Category]++
		case <-ctx.Done():
			metrics.TotalDuration = time.Since(start)
			return metrics
		}
	}

	metrics.TotalDuration = time.Since(start)
	return metrics
}

func generateCars(ctx context.Context, rng *rand.Rand, cat Category, n int, out chan<- Car) {
	// IDs únicos globales (para que no choquen entre categorías)
	for i := 0; i < n; i++ {
		id := int(atomic.AddInt64(&globalCarID, 1))
		car := Car{ID: id, Category: cat}

		// Llegadas algo aleatorias (simulación)
		sleep := time.Duration(rng.Intn(250)) * time.Millisecond
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return
		}

		select {
		case out <- car:
		case <-ctx.Done():
			return
		}
	}
}

var globalCarID int64

func dispatchStage(ctx context.Context, sm *StateManager, in stageInput, handle func(Car) bool) {
	for {
		car, ok := pickNext(ctx, sm, in)
		if !ok {
			// ctx cancelado o cierre
			return
		}
		if !handle(car) {
			return
		}
	}
}

func sendByCategory(ctx context.Context, car Car, a, b, c chan<- Car) {
	var dst chan<- Car
	switch car.Category {
	case CatA:
		dst = a
	case CatB:
		dst = b
	case CatC:
		dst = c
	default:
		dst = a
	}
	select {
	case dst <- car:
	case <-ctx.Done():
	}
}

func sleepWithJitter(ctx context.Context, rng *rand.Rand, base time.Duration, maxJitterSeconds int64) {
	j := time.Duration(0)
	if maxJitterSeconds > 0 {
		j = time.Duration(rng.Int63n(maxJitterSeconds+1)) * time.Second
	}
	t := base + j
	select {
	case <-time.After(t):
	case <-ctx.Done():
	}
}
