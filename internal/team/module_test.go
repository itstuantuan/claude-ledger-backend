package team

import "testing"

func TestValidateTeamMatchesFrontend(t *testing.T) {
	valid := Input{Name: "施工队", Leader: "张三", Phone: "13800138001", Status: "ACTIVE"}
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	valid.Status = "UNKNOWN"
	if err := validate(valid); err == nil {
		t.Fatal("invalid status accepted")
	}
}
