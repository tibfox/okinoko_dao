package contract

import (
	"encoding/json"
	"os"
)

type MockState struct {
	db       map[string]string
	filename string
}

func NewMockState() *MockState {
	return &MockState{
		db:       make(map[string]string),
		filename: "state.json",
	}
}

func (m *MockState) Set(key, value string) {
	m.db[key] = value
	if err := m.saveToFile(); err != nil {
		panic(err) // or log.Fatal(err)
	}
}

func (m *MockState) Get(key string) *string {
	val, ok := m.db[key]
	if !ok {
		return nil
	}
	return &val
}

func (m *MockState) Delete(key string) {
	delete(m.db, key)
	if err := m.saveToFile(); err != nil {
		panic(err)
	}
}

// saveToFile writes the full map to a JSON file
func (m *MockState) saveToFile() error {
	data, err := json.MarshalIndent(m.db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filename, data, 0644)
}

// LoadFromFile loads the map from a JSON file
func (m *MockState) LoadFromFile() {
	data, err := os.ReadFile(m.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return // file doesn't exist yet
		}
		panic(err)
	}
	if err := json.Unmarshal(data, &m.db); err != nil {
		panic(err)
	}
}
