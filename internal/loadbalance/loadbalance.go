package loadbalance

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"light-api-gateway/internal/config"
)

var ErrNoTargets = errors.New("no enabled targets")

type Picker interface {
	Next() (config.TargetConfig, bool)
}

func NewPicker(strategy string, targets []config.TargetConfig) (Picker, error) {
	enabled := make([]config.TargetConfig, 0, len(targets))
	for _, target := range targets {
		if !target.Enabled || target.URL == "" {
			continue
		}
		if strings.EqualFold(target.HealthStatus, "unhealthy") {
			continue
		}
		if target.Weight <= 0 {
			target.Weight = 1
		}
		enabled = append(enabled, target)
	}
	if len(enabled) == 0 {
		return nil, ErrNoTargets
	}

	switch strings.ToLower(strategy) {
	case "", "round-robin", "round_robin":
		return &roundRobinPicker{targets: enabled}, nil
	case "weighted", "weighted-round-robin", "weighted_round_robin":
		return newSmoothWeightedPicker(enabled), nil
	case "random":
		return &randomPicker{
			targets: enabled,
			rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
		}, nil
	default:
		return nil, errors.New("unsupported load balance strategy: " + strategy)
	}
}

type roundRobinPicker struct {
	targets []config.TargetConfig
	next    uint64
}

func (p *roundRobinPicker) Next() (config.TargetConfig, bool) {
	if len(p.targets) == 0 {
		return config.TargetConfig{}, false
	}
	index := atomic.AddUint64(&p.next, 1) - 1
	return p.targets[int(index%uint64(len(p.targets)))], true
}

type randomPicker struct {
	targets []config.TargetConfig
	rand    *rand.Rand
	mu      sync.Mutex
}

func (p *randomPicker) Next() (config.TargetConfig, bool) {
	if len(p.targets) == 0 {
		return config.TargetConfig{}, false
	}
	p.mu.Lock()
	index := p.rand.Intn(len(p.targets))
	p.mu.Unlock()
	return p.targets[index], true
}

type smoothWeightedPicker struct {
	targets []config.TargetConfig
	current []int
	total   int
	mu      sync.Mutex
}

func newSmoothWeightedPicker(targets []config.TargetConfig) Picker {
	total := 0
	for _, target := range targets {
		total += target.Weight
	}
	return &smoothWeightedPicker{
		targets: targets,
		current: make([]int, len(targets)),
		total:   total,
	}
}

func (p *smoothWeightedPicker) Next() (config.TargetConfig, bool) {
	if len(p.targets) == 0 {
		return config.TargetConfig{}, false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bestIndex := 0
	for i, target := range p.targets {
		p.current[i] += target.Weight
		if p.current[i] > p.current[bestIndex] {
			bestIndex = i
		}
	}
	p.current[bestIndex] -= p.total

	return p.targets[bestIndex], true
}
