package main

import (
	"errors"
	"sort"
)

type Pod struct {
	Name      string
	CPU       int
	Memory    int
	GPU       int
	TPURequst int
	Priority  int
}
type TPU struct {
	ID        int
	NUMANode  int
	Allocated bool
}
type Node struct {
	Name           string
	CPUCapacity    int
	MemoryCapacity int
	GPUCapacity    int
	TPUs           []TPU
	CPUUsed        int
	MemoryUsed     int
	GPUUsed        int
}

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
	TPU    int
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

type WeightScorePlugin struct {
	ScorePlugin
	Weight int
}
type Scheduler struct {
	prefilters []PreFilterPlugin
	filters    []FilterPlugin
	scorers    []WeightScorePlugin
}

func NewScheduler(perfilters []PreFilterPlugin, filters []FilterPlugin, scorers []WeightScorePlugin) *Scheduler {
	return &Scheduler{prefilters: perfilters, filters: filters, scorers: scorers}
}

func Schedule(pod Pod, nodes []Node) (string, error) {
	return NewScheduler(
		[]PreFilterPlugin{FooPreFilter{}},
		[]FilterPlugin{ResourceFitFilter{}, TPUFitFilter{}},
		[]WeightScorePlugin{
			{CPUAndGPUPackingScore{}, 1},
			{TPUTopologyScore{}, 10},
		},
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
		pod.TPURequst,
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

type TPUFitFilter struct{}

func (TPUFitFilter) Filter(pod Pod, node Node, status *CycleState) bool {
	needTPU := status.podrequest.TPU
	return caculateTPUAvailable(node) >= needTPU
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

type TPUTopologyScore struct{}

func (TPUTopologyScore) Score(pod Pod, node Node, status *CycleState) int {
	if status.podrequest.TPU == 0 {
		return 0
	}
	tpuNUMAMap := make(map[int]int)
	for _, tpu := range node.TPUs {
		if !tpu.Allocated {
			tpuNUMAMap[tpu.NUMANode]++
		}
	}
	list := make([]int, 0)
	for _, v := range tpuNUMAMap {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i] > list[j]
	})
	groups := 0
	sum := 0
	for i := range list {
		sum += list[i]
		groups++
		if sum >= status.podrequest.TPU {
			break
		}
	}
	return 100 - (groups - 1)
}

func percentage(value, capacity int) int {
	if capacity <= 0 {
		return 0
	}
	return value * 100 / capacity
}

func caculateTPUAvailable(node Node) int {
	cnt := 0
	tpus := node.TPUs

	for _, tpu := range tpus {
		if !tpu.Allocated {
			cnt++
		}
	}
	return cnt
}
