package main

import (
	"reflect"
	"testing"
)

// func Test_Lookup() {
// 	mh := getMaglevHash()
// 	mh.Lookup()
// }

func TestPopulate_NoEmptySlot(t *testing.T) {
	mh := getMaglevHash()
	for _, b := range table.lookup {
		if b == -1 {
			t.Fatalf("slot %d is empty", b)
		}
	}
}

func TestPopulate_Deterministic(t *testing.T) {
	mh1 := getMaglevHash()
	mh2 := getMaglevHash()
	if !reflect.DeepEqual(mh1, mh2) {
		t.Fatalf("populate is not deterministic")
	}
}

func TestRevive(t *testing.T) {
	mh := getMaglevHash()
	mh.Kill(0)
	mh.Revive(0)
	if len(mh.dead) != 0 {
		t.Fatalf("backend 0 is not revived")
	}
}

func TestKill(t *testing.T) {
	mh := getMaglevHash()
	mh.Kill(0)
	if len(mh.dead) != 1 {
		t.Fatalf("backend 0 is not killed")
	}

	mh.Kill(1)
	if len(mh.dead) != 2 {
		t.Fatalf("backend 1 is not killed")
	}
}

func getMaglevHash() *MaglevHash {
	return New([]*Backend{
		&Backend{
			Name: "backend-1",
		},
		&Backend{
			Name: "backend-2",
		},
		&Backend{
			Name: "backend-3",
		},
	}, 13)
}

