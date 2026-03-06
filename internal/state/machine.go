package state

type Value string

const (
	Idle       Value = "Idle"
	Recording  Value = "Recording"
	Processing Value = "Processing"
	Inserting  Value = "Inserting"
)

type Machine struct {
	current Value
}

func New() *Machine {
	return &Machine{current: Idle}
}

func (m *Machine) Current() Value {
	return m.current
}

func (m *Machine) Toggle() Value {
	switch m.current {
	case Idle:
		m.current = Recording
	case Recording:
		m.current = Processing
	}
	return m.current
}

func (m *Machine) BeginInsert() Value {
	if m.current == Processing {
		m.current = Inserting
	}
	return m.current
}

func (m *Machine) Reset() Value {
	m.current = Idle
	return m.current
}
