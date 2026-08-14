package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ripper19/simulator/internal/queue"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// RunJob executes a run_simulation job and publishes its result. It returns an
// error only for setup failures (bad payload, unknown model, init error);
// simulation runtime failures are captured in the published result so the job
// itself is considered processed (the worker did its work).
func RunJob(ctx context.Context, reg *registry.Registry, q *queue.Queue, job queue.Job) error {
	var p queue.RunJobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("workers: bad payload: %w", err)
	}

	entry, ok := reg.Get(p.ModelID)
	if p.ModelVersion != "" {
		if v, found := reg.GetVersion(p.ModelID, p.ModelVersion); found {
			entry = v
		}
	}
	if !ok {
		return fmt.Errorf("workers: unknown model %q", p.ModelID)
	}
	if entry.Factory == nil {
		return fmt.Errorf("workers: model %q has no factory", p.ModelID)
	}

	mdl := entry.Factory()
	if len(p.Config) > 0 {
		if cm, ok := mdl.(simulation.ConfigurableModel); ok {
			if err := cm.Configure(p.Config); err != nil {
				return fmt.Errorf("workers: configure: %w", err)
			}
		}
	}

	mode := mdl.Metadata().Mode
	switch p.Mode {
	case "tick":
		mode = model.ModeTick
	case "event":
		mode = model.ModeEvent
	}

	sim, err := simulation.New(ctx, simulation.Config{
		ID:       p.SimulationID,
		Seed:     p.Seed,
		Mode:     mode,
		MaxTicks: p.MaxTicks,
		MaxTime:  p.MaxTime,
		Workers:  p.Workers,
	}, mdl)
	if err != nil {
		return fmt.Errorf("workers: init simulation: %w", err)
	}

	runErr := sim.Run(ctx)

	result := queue.Result{
		JobID:        job.ID,
		SimulationID: p.SimulationID,
		Status:       "completed",
		CompletedAt:  time.Now(),
	}
	if runErr != nil {
		result.Status = "failed"
		result.Error = runErr.Error()
	}
	if snap, err := sim.Snapshot(); err == nil {
		if b, err := json.Marshal(snap); err == nil {
			result.Snapshot = b
		}
	}
	return q.PublishResult(ctx, result)
}
