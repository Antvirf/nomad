package procstats

import (
	"time"

	"github.com/hashicorp/go-set"
	"github.com/hashicorp/nomad/client/lib/cpustats"
	"github.com/hashicorp/nomad/plugins/drivers"
)

var (
	// The statistics the basic executor exposes
	ExecutorBasicMeasuredMemStats = []string{"RSS", "Swap"}
	ExecutorBasicMeasuredCpuStats = []string{"System Mode", "User Mode", "Percent"}
)

type ProcessID uint32

// ProcUsages is a map from PID to the resources that process is consuming.
//
// The pid type is a string for some reason.
type ProcUsages map[string]*drivers.ResourceUsage

type ProcessStats interface {
	StatProcesses() ProcUsages
}

type ProcessList interface {
	ListProcesses() *set.Set[ProcessID]
}

func Aggregate(systemStats *cpustats.Tracker, procStats ProcUsages) *drivers.TaskResourceUsage {
	ts := time.Now().UTC().UnixNano()
	var (
		systemModeCPU, userModeCPU, percent float64
		totalRSS, totalSwap                 uint64
	)

	for _, pidStat := range procStats {
		systemModeCPU += pidStat.CpuStats.SystemMode
		userModeCPU += pidStat.CpuStats.UserMode
		percent += pidStat.CpuStats.Percent

		totalRSS += pidStat.MemoryStats.RSS
		totalSwap += pidStat.MemoryStats.Swap
	}

	totalCPU := &drivers.CpuStats{
		SystemMode: systemModeCPU,
		UserMode:   userModeCPU,
		Percent:    percent,
		Measured:   ExecutorBasicMeasuredCpuStats,
		TotalTicks: systemStats.TicksConsumed(percent),
	}

	totalMemory := &drivers.MemoryStats{
		RSS:      totalRSS,
		Swap:     totalSwap,
		Measured: ExecutorBasicMeasuredMemStats,
	}

	resourceUsage := drivers.ResourceUsage{
		MemoryStats: totalMemory,
		CpuStats:    totalCPU,
	}
	return &drivers.TaskResourceUsage{
		ResourceUsage: &resourceUsage,
		Timestamp:     ts,
		Pids:          procStats,
	}
}
