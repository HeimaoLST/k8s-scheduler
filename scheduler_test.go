package main

import "testing"

func TestScheduleChoosesNodeWithLessGPUFragmentation(t *testing.T) {
	nodes := []Node{
		{Name: "node-a", CPUCapacity: 32, MemoryCapacity: 64, GPUCapacity: 8, CPUUsed: 20, GPUUsed: 6},
		{Name: "node-b", CPUCapacity: 64, MemoryCapacity: 64, GPUCapacity: 8, CPUUsed: 40, GPUUsed: 2},
	}

	name, err := Schedule(Pod{Name: "workload", CPU: 4, Memory: 1, GPU: 2}, nodes)
	if err != nil {
		t.Fatalf("Schedule returned an unexpected error: %v", err)
	}
	if name != "node-a" {
		t.Fatalf("Schedule selected %q, want node-a", name)
	}
}

func TestScheduleReturnsErrorWhenNoNodeFits(t *testing.T) {
	_, err := Schedule(Pod{CPU: 9}, []Node{{Name: "node-a", CPUCapacity: 8}})
	if err == nil {
		t.Fatal("Schedule returned nil error when no node fits")
	}
}

func TestScheduleKeepsInputOrderForEqualScores(t *testing.T) {
	nodes := []Node{
		{Name: "node-a", CPUCapacity: 8, MemoryCapacity: 8},
		{Name: "node-b", CPUCapacity: 8, MemoryCapacity: 8},
	}

	name, err := Schedule(Pod{CPU: 1}, nodes)
	if err != nil {
		t.Fatalf("Schedule returned an unexpected error: %v", err)
	}
	if name != "node-a" {
		t.Fatalf("Schedule selected %q, want node-a", name)
	}
}
