package project

import "testing"

func TestProjectRequiresWorkerAndOrderedDates(t *testing.T) {
	start, end := "2026-09-20", "2026-09-01"
	input := Input{Name: "项目", Address: "地址", Manager: "张三", WorkerIDs: []string{"worker"}, StartDate: &start, EndDate: &end, Status: "ACTIVE"}
	if _, _, err := validate(input); err == nil {
		t.Fatal("reversed dates accepted")
	}
	input.EndDate = nil
	input.WorkerIDs = nil
	if _, _, err := validate(input); err == nil {
		t.Fatal("empty workers accepted")
	}
}
