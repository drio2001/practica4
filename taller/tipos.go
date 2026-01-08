// types.go
package main

import (
	"flag"
	"math/rand"
	"time"
)

type Category int

const (
	CatA Category = iota
	CatB
	CatC
)

func (c Category) String() string {
	switch c {
	case CatA:
		return "A" // mecánica (alta)
	case CatB:
		return "B" // eléctrica (media)
	case CatC:
		return "C" // carrocería (baja)
	default:
		return "?"
	}
}

type Phase int

const (
	Phase1Docs     Phase = 1
	Phase2Mechanic Phase = 2
	Phase3Clean    Phase = 3
	Phase4Delivery Phase = 4
)

type Car struct {
	ID       int
	Category Category
}

type Config struct {
	ServerAddr string

	// Carga de trabajo
	NumA int
	NumB int
	NumC int

	// Recursos
	NumPlazas    int
	NumMecanicos int

	// Colas máximas por fase (espera)
	MaxWaitP1 int
	MaxWaitP2 int
	MaxWaitP3 int
	MaxWaitP4 int

	// Recursos fase 3 y 4 (para modelar orden/prioridad también ahí)
	NumCleaners  int
	NumReviewers int

	// Variación: se suma [0..MaxJitterSeconds] a cada fase
	MaxJitterSeconds int64

	// Semilla aleatoria (0 => auto)
	Seed int64
}

func DefaultConfig() Config {
	return Config{
		ServerAddr: "localhost:8000",

		NumA: 10, NumB: 10, NumC: 10,

		NumPlazas:    6,
		NumMecanicos: 3,

		MaxWaitP1: 20,
		MaxWaitP2: 20,
		MaxWaitP3: 20,
		MaxWaitP4: 20,

		NumCleaners:  1,
		NumReviewers: 1,

		MaxJitterSeconds: 2,
		Seed:             0,
	}
}

func (cfg *Config) ApplyEnvOrArgs(args []string) {
	fs := flag.NewFlagSet("taller", flag.ContinueOnError)

	fs.StringVar(&cfg.ServerAddr, "addr", cfg.ServerAddr, "dirección del servidor (tcp)")
	fs.IntVar(&cfg.NumA, "a", cfg.NumA, "número de coches categoría A")
	fs.IntVar(&cfg.NumB, "b", cfg.NumB, "número de coches categoría B")
	fs.IntVar(&cfg.NumC, "c", cfg.NumC, "número de coches categoría C")

	fs.IntVar(&cfg.NumPlazas, "plazas", cfg.NumPlazas, "número de plazas de espera (fase 1)")
	fs.IntVar(&cfg.NumMecanicos, "mecanicos", cfg.NumMecanicos, "número de mecánicos (fase 2)")

	fs.IntVar(&cfg.MaxWaitP1, "q1", cfg.MaxWaitP1, "cola máxima fase 1")
	fs.IntVar(&cfg.MaxWaitP2, "q2", cfg.MaxWaitP2, "cola máxima fase 2")
	fs.IntVar(&cfg.MaxWaitP3, "q3", cfg.MaxWaitP3, "cola máxima fase 3")
	fs.IntVar(&cfg.MaxWaitP4, "q4", cfg.MaxWaitP4, "cola máxima fase 4")

	fs.IntVar(&cfg.NumCleaners, "cleaners", cfg.NumCleaners, "número de limpiadorxs (fase 3)")
	fs.IntVar(&cfg.NumReviewers, "reviewers", cfg.NumReviewers, "número de revisorxs (fase 4)")

	fs.Int64Var(&cfg.MaxJitterSeconds, "jitter", cfg.MaxJitterSeconds, "variación máxima (seg) por fase")
	fs.Int64Var(&cfg.Seed, "seed", cfg.Seed, "semilla RNG (0 => auto)")

	_ = fs.Parse(args)
}

func (cfg Config) Rand() *rand.Rand {
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}

// Tiempo base por fase, según enunciado:
// A: 5s, B: 3s, C: 1s. :contentReference[oaicite:6]{index=6}
func BaseSeconds(cat Category) time.Duration {
	switch cat {
	case CatA:
		return 5 * time.Second
	case CatB:
		return 3 * time.Second
	case CatC:
		return 1 * time.Second
	default:
		return 1 * time.Second
	}
}
