// Package app wires the compiled-in example models into the platform's model
// registry, shared by the API and worker entrypoints.
package app

import (
	"github.com/ripper19/simulator/examples/counter"
	dsystem "github.com/ripper19/simulator/examples/distributed-system"
	"github.com/ripper19/simulator/examples/economy"
	"github.com/ripper19/simulator/examples/ecosystem"
	"github.com/ripper19/simulator/examples/logistics"
	queueing "github.com/ripper19/simulator/examples/queue"
	"github.com/ripper19/simulator/examples/traffic"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/pkg/simulation"
)

// RegisterModels registers every compiled-in example model.
func RegisterModels(reg *registry.Registry) {
	reg.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model { return &counter.CounterWorld{} })
	reg.Register((&traffic.Traffic{}).Metadata(), func() simulation.Model { return &traffic.Traffic{} })
	reg.Register((&economy.Economy{}).Metadata(), func() simulation.Model { return &economy.Economy{} })
	reg.Register((&ecosystem.Ecosystem{}).Metadata(), func() simulation.Model { return &ecosystem.Ecosystem{} })
	reg.Register((&queueing.Queueing{}).Metadata(), func() simulation.Model { return &queueing.Queueing{} })
	reg.Register((&dsystem.DistributedSystem{}).Metadata(), func() simulation.Model { return &dsystem.DistributedSystem{} })
	reg.Register((&logistics.Logistics{}).Metadata(), func() simulation.Model { return &logistics.Logistics{} })
}
