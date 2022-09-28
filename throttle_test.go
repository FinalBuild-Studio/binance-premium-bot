package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/ratelimit"
)

func TestThrottle(t *testing.T) {
	rl := ratelimit.New(1) // per second

	prev := time.Now()
	for i := 0; i < 10; i++ {
		now := rl.Take()

		assert.Equal(t, prev.Unix()+int64(i), now.Unix())
	}
}
