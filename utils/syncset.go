package utils

import (
	"encoding/json"
	"sync"
)

type SyncSet[K comparable] struct {
	m sync.Map
}

func (m *SyncSet[K]) Delete(key K) { m.m.Delete(key) }

func (m *SyncSet[K]) Exists(key K) bool {
	_, exists := m.m.Load(key)
	return exists
}

func (m *SyncSet[K]) Range(f func(key K) bool) {
	m.m.Range(func(key, value any) bool { return f(key.(K)) })
}

func (m *SyncSet[K]) Add(key K) { m.m.Store(key, nil) }

func (m *SyncSet[K]) Len() int {
	// TOOD: 性能优化
	times := 0
	m.Range(func(_ K) bool {
		times++
		return true
	})
	return times
}

func (m *SyncSet[K]) MarshalJSON() ([]byte, error) {
	m2 := []K{}
	m.Range(func(key K) bool {
		m2 = append(m2, key)
		return true
	})
	return json.Marshal(m2)
}

func (m *SyncSet[K]) UnmarshalJSON(b []byte) error {
	m2 := []K{}
	err := json.Unmarshal(b, &m2)
	if err != nil {
		return err
	}
	for _, k := range m2 {
		m.Add(k)
	}
	return nil
}

func (m *SyncSet[K]) ToArray() []K {
	var m2 []K
	m.Range(func(key K) bool {
		m2 = append(m2, key)
		return true
	})
	return m2
}
