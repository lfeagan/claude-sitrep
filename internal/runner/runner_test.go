package runner

import (
	"context"
	"errors"
	"testing"
)

func TestFake(t *testing.T) {
	f := Fake{"ps -o lstart= -p 7": {Out: []byte("Mon Nov 20 10:00:00 2026\n")}, "tmux ls": {Err: errors.New("no server")}}
	out, err := f.Run(context.Background(), "ps", "-o", "lstart=", "-p", "7")
	if err != nil || string(out) != "Mon Nov 20 10:00:00 2026\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := f.Run(context.Background(), "tmux", "ls"); err == nil {
		t.Fatal("want the canned error")
	}
	if _, err := f.Run(context.Background(), "cmux", "top"); err == nil {
		t.Fatal("want an error for an unknown command")
	}
}

func TestExec(t *testing.T) {
	out, err := Exec{}.Run(context.Background(), "echo", "hi")
	if err != nil || string(out) != "hi\n" {
		t.Fatalf("got %q, %v", out, err)
	}
}
