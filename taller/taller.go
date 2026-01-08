// taller.go
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

var (
	buf    bytes.Buffer
	logger = log.New(&buf, "logger: ", log.Lshortfile)
)

func main() {

	// Usa DefaultConfig de tipos.go
	// Usa StateManager de estados.go
	// Usa Metrics de metricas.go
	// Usa RunSimulation de simulacion.go

	cfg := DefaultConfig()
	cfg.ApplyEnvOrArgs(os.Args[1:])

	conn, err := net.Dial("tcp", cfg.ServerAddr)
	if err != nil {
		logger.Fatal(err)
	}
	defer conn.Close()

	// Cancelación limpia por Ctrl+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Estado controlado por mutua (0..9)
	state := NewStateManager()
	defer state.Close()

	// Canal de entrada de estados (parseados desde el servidor)
	stateUpdates := make(chan int, 16)
	go func() {
		defer close(stateUpdates)
		readLoop(ctx, conn, stateUpdates)
	}()

	// Conectamos stateUpdates -> StateManager
	go func() {
		for s := range stateUpdates {
			state.SetState(s)
		}
	}()

	// Ejecutamos simulación (una vez)
	fmt.Println("=== Taller arrancado ===")
	fmt.Printf("Configuracion: Plazas=%d || mecanicos=%d A=%d - B=%d - C=%d\n",
		cfg.NumPlazas, cfg.NumMecanicos, cfg.NumA, cfg.NumB, cfg.NumC)

	metrics := RunSimulation(ctx, cfg, state)

	fmt.Println("=== Taller finalizado ===")
	fmt.Printf("Total coches procesados: %d\n", metrics.TotalProcessed)
	fmt.Printf("Tiempo total: %s\n", metrics.TotalDuration)
	fmt.Printf("Por categoría: A=%d B=%d C=%d\n", metrics.ByCategory[CatA], metrics.ByCategory[CatB], metrics.ByCategory[CatC])
}

// readLoop lee líneas del servidor. Si la línea es un entero 0..9, emite a stateUpdates.
// Si no lo es (mensajes tipo "Taller localizado..."), lo ignora.
func readLoop(ctx context.Context, conn net.Conn, stateUpdates chan<- int) {
	sc := bufio.NewScanner(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !sc.Scan() {
			if err := sc.Err(); err != nil && err != io.EOF {
				fmt.Println("error leyendo servidor:", err)
			}
			return
		}

		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		n, err := strconv.Atoi(line)
		if err != nil {
			// Mensaje no numérico (p.ej. "Taller localizado ...")
			continue
		}
		if n < 0 || n > 9 {
			continue
		}
		stateUpdates <- n
	}
}
