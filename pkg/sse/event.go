package sse

import (
	"strconv"
	"sync"
	"time"
)

var eventPool = sync.Pool{
	New: func() interface{} {
		return &Event{}
	},
}

type Event struct {
	ID    string
	Type  string
	Data  []byte
	Retry time.Duration

	bitset uint8
}

const (
	isSetID uint8 = 1 << iota
	isSetType
	isSetData
	isSetRetry
)

func NewEvent() *Event {
	event := eventPool.Get().(*Event)
	event.reset()
	return event
}

func (e *Event) reset() {
	e.ID = ""
	e.Type = ""
	e.Data = nil
	e.Retry = 0
	e.bitset = 0
}

func (e *Event) Release() {
	e.reset()
	eventPool.Put(e)
}

func (e *Event) Clone() *Event {
	newEvent := NewEvent()
	newEvent.ID = e.ID
	newEvent.Type = e.Type
	newEvent.Data = make([]byte, len(e.Data))
	copy(newEvent.Data, e.Data)
	newEvent.Retry = e.Retry
	newEvent.bitset = e.bitset
	return newEvent
}

func (e *Event) SetID(id string) {
	e.ID = id
	e.bitset |= isSetID
}

func (e *Event) SetEvent(eventType string) {
	e.Type = eventType
	e.bitset |= isSetType
}

func (e *Event) SetData(data []byte) {
	e.Data = data
	e.bitset |= isSetData
}

func (e *Event) SetDataString(data string) {
	e.Data = []byte(data)
	e.bitset |= isSetData
}

func (e *Event) AppendData(data []byte) {
	e.Data = append(e.Data, data...)
	e.bitset |= isSetData
}

func (e *Event) AppendDataString(data string) {
	e.Data = append(e.Data, []byte(data)...)
	e.bitset |= isSetData
}

func (e *Event) SetRetry(retry time.Duration) {
	e.Retry = retry
	e.bitset |= isSetRetry
}

func (e *Event) IsSetID() bool {
	return e.bitset&isSetID != 0
}

func (e *Event) IsSetType() bool {
	return e.bitset&isSetType != 0
}

func (e *Event) IsSetData() bool {
	return e.bitset&isSetData != 0
}

func (e *Event) IsSetRetry() bool {
	return e.bitset&isSetRetry != 0
}

func (e *Event) String() string {
	var result []byte

	if e.IsSetID() {
		result = append(result, "id: "...)
		result = append(result, e.ID...)
		result = append(result, '\n')
	}

	if e.IsSetType() {
		result = append(result, "event: "...)
		result = append(result, e.Type...)
		result = append(result, '\n')
	}

	if e.IsSetData() {
		lines := splitLines(e.Data)
		for _, line := range lines {
			result = append(result, "data: "...)
			result = append(result, line...)
			result = append(result, '\n')
		}
	}

	if e.IsSetRetry() {
		result = append(result, "retry: "...)
		result = append(result, strconv.FormatInt(e.Retry.Milliseconds(), 10)...)
		result = append(result, '\n')
	}

	result = append(result, '\n')
	return string(result)
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return [][]byte{nil}
	}

	var lines [][]byte
	start := 0

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		} else if data[i] == '\r' {
			lines = append(lines, data[start:i])
			if i+1 < len(data) && data[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}

	if start < len(data) {
		lines = append(lines, data[start:])
	} else if start == len(data) && len(lines) > 0 {
		lines = append(lines, nil)
	}

	return lines
}
