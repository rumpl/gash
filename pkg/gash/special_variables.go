package gash

import (
	"crypto/rand"
	"encoding/binary"
	"sync/atomic"

	"mvdan.cc/sh/v3/expand"
)

type specialVariableEnviron struct {
	base expand.Environ
	jobs *jobState
}

var randomFallback atomic.Uint32

func (e specialVariableEnviron) Get(name string) expand.Variable {
	if name == lastBackgroundVariable && e.jobs != nil {
		if pid := e.jobs.lastPID(); pid != "" {
			return expand.Variable{Kind: expand.String, Str: pid}
		}
	}
	if name != "RANDOM" {
		return e.base.Get(name)
	}
	var bytes [2]byte
	var value uint16
	if _, err := rand.Read(bytes[:]); err == nil {
		value = binary.LittleEndian.Uint16(bytes[:])
	} else {
		value = uint16(randomFallback.Add(1))
	}
	return expand.Variable{Kind: expand.String, Str: randomDecimal(value & 0x7fff)}
}

func (e specialVariableEnviron) Each(yield func(string, expand.Variable) bool) {
	e.base.Each(yield)
}

func randomDecimal(value uint16) string {
	if value == 0 {
		return "0"
	}
	var buffer [5]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}
