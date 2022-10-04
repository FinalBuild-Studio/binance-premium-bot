package main

import (
	"testing"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
)

func TestMessage(t *testing.T) {
	ch := make(chan models.EventMessage)

	go func() {
		messages := func() <-chan models.EventMessage {
			return ch
		}
		for v := range messages() {
			t.Log(v)
		}
	}()

	ch <- models.EventMessage{}
}
