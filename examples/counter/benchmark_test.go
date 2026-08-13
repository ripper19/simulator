package counter

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// BenchmarkCounterTick measures a single tick of CounterWorld over N entities
// for serial (workers=1) and parallel (workers=GOMAXPROCS) execution.
//
// Results must be interpreted together with hardware, OS, and Go version; run
// with:
//
//	go test -run '^$' -bench BenchmarkCounterTick -benchmem ./examples/counter
func BenchmarkCounterTick(b *testing.B) {
	sizes := []int{10_000, 100_000, 1_000_000}
	workerSets := []struct {
		name string
		n    int
	}{
		{"serial", 1},
		{"parallel", runtime.GOMAXPROCS(0)},
	}
	ctx := context.Background()
	for _, n := range sizes {
		for _, ws := range workerSets {
			b.Run(fmt.Sprintf("n=%d/%s", n, ws.name), func(b *testing.B) {
				m := &CounterWorld{N: n}
				sim, err := simulation.New(ctx, simulation.Config{
					ID:      "bench",
					Seed:    12345,
					Mode:    model.ModeTick,
					Workers: ws.n,
				}, m)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := sim.Step(ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
