package state

import "testing"

func TestMachineToggleTransitions(t *testing.T) {
	machine := New()

	if got := machine.Current(); got != Idle {
		t.Fatalf("initial state = %s, want %s", got, Idle)
	}

	if got := machine.Toggle(); got != Recording {
		t.Fatalf("Idle toggle = %s, want %s", got, Recording)
	}

	if got := machine.Toggle(); got != Processing {
		t.Fatalf("Recording toggle = %s, want %s", got, Processing)
	}

	if got := machine.Toggle(); got != Processing {
		t.Fatalf("Processing toggle = %s, want %s", got, Processing)
	}

	if got := machine.BeginInsert(); got != Inserting {
		t.Fatalf("BeginInsert = %s, want %s", got, Inserting)
	}

	if got := machine.Toggle(); got != Inserting {
		t.Fatalf("Inserting toggle = %s, want %s", got, Inserting)
	}

	if got := machine.Reset(); got != Idle {
		t.Fatalf("Reset = %s, want %s", got, Idle)
	}
}
