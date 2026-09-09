package main

import (
	"errors"
)

type Pod struct {
	Name     string
	CPU      int
	Memory   int
	GPU      int
	Priority int
}

type Node struct {
	Name           string
	CPUCapacity    int
	MemoryCapacity int
	GPUCapacity    int

	CPUUsed    int
	MemoryUsed int
	GPUUsed    int
}

// NodeResourcesAvailable is the result of the PreFilter resource calculation.
type NodeResourcesAvailable struct {
	CPU    int
	Memory int
	GPU    int
}
type CycleState struct {
	podrequest PodResources
}
type PodResources struct {
	CPU    int
	Memory int
	GPU    int
}

func availableResources(node Node) NodeResourcesAvailable {
	return NodeResourcesAvailable{
		CPU:    node.CPUCapacity - node.CPUUsed,
		Memory: node.MemoryCapacity - node.MemoryUsed,
		GPU:    node.GPUCapacity - node.GPUUsed,
	}
}

type PreFilterPlugin interface {
	PreFilter(pod Pod, status *CycleState) error
}
type FilterPlugin interface {
	Filter(pod Pod, node Node, status *CycleState) bool
}
type ScorePlugin interface {
	Score(pod Pod, node Node, status *CycleState) int
}

// Scheduler runs PreFilter, Filter, Score, and SelectNode in that order.
type Scheduler struct {
	prefilters []PreFilterPlugin
	filters    []FilterPlugin
	scorers    []ScorePlugin
}

func NewScheduler(perfilters []PreFilterPlugin, filters []FilterPlugin, scorers []ScorePlugin) *Scheduler {
	return &Scheduler{prefilters: perfilters, filters: filters, scorers: scorers}
}

// Schedule is the minimal scheduler requested by the exercise. It uses the
// default plugins; the score calculation itself remains inside ScorePlugin.
func Schedule(pod Pod, nodes []Node) (string, error) {
	return NewScheduler(
		[]PreFilterPlugin{FooPreFilter{}},
		[]FilterPlugin{ResourceFitFilter{}},
		[]ScorePlugin{CPUAndGPUPackingScore{weight: 10}},
	).Schedule(pod, nodes)
}

func (s *Scheduler) Schedule(pod Pod, nodes []Node) (string, error) {
	if pod.CPU < 0 || pod.Memory < 0 || pod.GPU < 0 {
		return "", errors.New("pod resource requests must not be negative")
	}
	status := &CycleState{}
	err := s.doPreFilters(pod, status)
	if err != nil {
		return "", errors.Join(errors.New("prefilter faild: "), err)
	}

	feasible := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if s.passesFilters(pod, node, status) {
			feasible = append(feasible, node)
		}
	}
	if len(feasible) == 0 {
		return "", errors.New("no feasible node")
	}

	// Score: every scorer contributes to the total score.
	type nodeWithScore struct {
		name  string
		score int
	}
	scored := make([]nodeWithScore, 0, len(feasible))
	for _, node := range feasible {
		scored = append(scored, nodeWithScore{name: node.Name, score: s.score(pod, node, status)})
	}

	resIndex := 0
	maxScore := -1
	for i := range scored {
		if scored[i].score > maxScore {
			resIndex = i
			maxScore = scored[i].score
		}
	}
	return scored[resIndex].name, nil
}

func (s *Scheduler) doPreFilters(pod Pod, status *CycleState) error {
	for _, prefilter := range s.prefilters {
		err := prefilter.PreFilter(pod, status)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) passesFilters(pod Pod, node Node, status *CycleState) bool {
	for _, filter := range s.filters {
		if !filter.Filter(pod, node, status) {
			return false
		}
	}
	return true
}

func (s *Scheduler) score(pod Pod, node Node, status *CycleState) int {
	total := 0
	for _, scorer := range s.scorers {
		total += scorer.Score(pod, node, status)
	}
	return total
}

type FooPreFilter struct{}

func (FooPreFilter) PreFilter(pod Pod, status *CycleState) error {
	status.podrequest = PodResources{
		pod.CPU,
		pod.Memory,
		pod.GPU,
	}
	return nil
}

// ResourceFitFilter rejects nodes that cannot satisfy all requested resources.
type ResourceFitFilter struct{}

func (ResourceFitFilter) Filter(pod Pod, node Node, status *CycleState) bool {
	podrequest := status.podrequest
	available := availableResources(node)
	return available.CPU >= podrequest.CPU &&
		available.Memory >= podrequest.Memory &&
		available.GPU >= podrequest.GPU
}

type CPUAndGPUPackingScore struct {
	weight int
}

func (s CPUAndGPUPackingScore) Score(pod Pod, node Node, status *CycleState) int {
	available := availableResources(node)
	cpuScore := percentage(available.CPU-pod.CPU, node.CPUCapacity)

	if node.GPUCapacity == 0 {
		return cpuScore
	}
	gpuPackingScore := 100 - percentage(available.GPU-pod.GPU, node.GPUCapacity)
	return (cpuScore + gpuPackingScore) * s.weight
}

func percentage(value, capacity int) int {
	if capacity <= 0 {
		return 0
	}
	return value * 100 / capacity
}
