package worker

import "testing"

func TestValidateWorkerMatchesFrontend(t *testing.T) {
	valid := Input{Name: "张三", Phone: "13900139001", Status: "ACTIVE"}
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	valid.Phone = "123"
	if err := validate(valid); err == nil {
		t.Fatal("invalid phone accepted")
	}
}
