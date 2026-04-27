package cli

import (
	"context"
	"errors"
	"testing"
)

func TestValidateFlag(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// valid single-char shorts
		{"-i", false},
		{"-u", false},
		{"-v", false},
		{"-h", false},
		// valid longs
		{"--install", false},
		{"--update-geo", false},
		{"--help", false},
		{"--version", false},
		// non-flag (positional)
		{"install", false},
		{"proxy", false},
		// invalid multi-char shorts
		{"-ia", true},
		{"-ug", true},
		{"-dk", true},
		{"-ri", true},
		{"-uk", true},
		{"-tp", true},
		{"-ad", true},
		{"-af", true},
		{"-toff", true},
		{"-xbr", true},
		{"-ugc", true},
		// degenerate cases
		{"-", true},           // just a dash, len == 1 → not a flag but still weird
		{"--", true},          // incomplete long
		{"---install", false}, // technically len > 2 with --, treated as long; no rule
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validateFlag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFlag(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDispatch_UnknownCommand(t *testing.T) {
	// clear registry for isolation
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	err := Dispatch(context.Background(), []string{"--nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestDispatch_MultiCharShortRejected(t *testing.T) {
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	err := Dispatch(context.Background(), []string{"-ia"})
	if err == nil {
		t.Fatal("expected error for multi-char short flag")
	}
}

func TestDispatch_RoutesByShort(t *testing.T) {
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	called := false
	Register(Cmd{Short: "-x", Long: "--xtest", Handler: func(_ context.Context, _ []string) error {
		called = true
		return nil
	}})

	if err := Dispatch(context.Background(), []string{"-x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler not called via short flag")
	}
}

func TestDispatch_RoutesByLong(t *testing.T) {
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	called := false
	Register(Cmd{Short: "-x", Long: "--xtest", Handler: func(_ context.Context, _ []string) error {
		called = true
		return nil
	}})

	if err := Dispatch(context.Background(), []string{"--xtest"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler not called via long flag")
	}
}

func TestDispatch_HandlerErrorPropagates(t *testing.T) {
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	sentinel := errors.New("handler boom")
	Register(Cmd{Long: "--boom", Handler: func(_ context.Context, _ []string) error {
		return sentinel
	}})

	err := Dispatch(context.Background(), []string{"--boom"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestDispatch_NoArgs_ShowsHelp(t *testing.T) {
	saved := registry
	registry = nil
	defer func() { registry = saved }()

	// showHelp returns nil — just check no panic and no error
	err := Dispatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error on empty args: %v", err)
	}
}
