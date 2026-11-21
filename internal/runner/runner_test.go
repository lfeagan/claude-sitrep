package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFake(t *testing.T) {
	f := &Fake{Answers: map[string]Result{"ps -o lstart= -p 7": {Out: []byte("Mon Nov 20 10:00:00 2026\n")}, "tmux ls": {Err: errors.New("no server")}}}
	out, err := f.Run(context.Background(), Cmd{Name: "ps", Args: []string{"-o", "lstart=", "-p", "7"}})
	if err != nil || string(out) != "Mon Nov 20 10:00:00 2026\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := f.Run(context.Background(), Cmd{Name: "tmux", Args: []string{"ls"}}); err == nil {
		t.Fatal("want the canned error")
	}
	if _, err := f.Run(context.Background(), Cmd{Name: "cmux", Args: []string{"top"}}); err == nil {
		t.Fatal("want an error for an unknown command")
	}
	if calls := f.Calls(); len(calls) != 3 || calls[0].Name != "ps" {
		t.Fatalf("calls %v", calls)
	}
}

func TestExec(t *testing.T) {
	out, err := Exec{}.Run(context.Background(), Cmd{Name: "echo", Args: []string{"hi"}})
	if err != nil || string(out) != "hi\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	out, err = Exec{}.Run(context.Background(), Cmd{Name: "sh", Args: []string{"-c", "echo $X; pwd; cat"}, Dir: "/", Env: []string{"X=1"}, Stdin: strings.NewReader("in")})
	if err != nil || string(out) != "1\n/\nin" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestExecDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Exec{}.Run(ctx, Cmd{Name: "sh", Args: []string{"-c", "sleep 30 & wait"}})
	if err == nil || time.Since(start) > 10*time.Second {
		t.Fatalf("the deadline did not stop the command: %v after %v", err, time.Since(start))
	}
}
