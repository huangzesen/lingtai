package main

import (
	"os"
	"strings"
	"testing"
)

func TestPresetRevisionDispatchPrecedesBootstrap(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	dispatch := strings.Index(body, `headless.RunPresetRevision(os.Args[3:]`)
	bootstrap := strings.Index(body, `preset.Bootstrap(globalDir)`)
	if dispatch < 0 {
		t.Fatal("presets revise dispatch is missing")
	}
	if bootstrap < 0 {
		t.Fatal("existing preset bootstrap boundary is missing")
	}
	if dispatch > bootstrap {
		t.Fatalf("revision dispatch offset %d occurs after preset bootstrap offset %d", dispatch, bootstrap)
	}
	if !strings.Contains(body, `len(os.Args) > 2 && os.Args[2] == "revise"`) {
		t.Fatal("revision dispatch is not guarded by the presets revise subcommand")
	}
}
