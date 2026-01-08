// metrics.go
package main

import "time"

type Metrics struct {
	TotalProcessed int
	TotalDuration  time.Duration
	ByCategory     map[Category]int
}

func NewMetrics() Metrics {
	return Metrics{
		ByCategory: map[Category]int{
			CatA: 0,
			CatB: 0,
			CatC: 0,
		},
	}
}
