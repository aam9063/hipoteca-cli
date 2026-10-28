package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The full type → exit code table of the arranque, section 6.4. One row per
// named type; 130 (signal) is not an envelope type and is not here.
func TestExitCodeTable(t *testing.T) {
	tests := []struct {
		kind Kind
		want int
	}{
		{KindInternal, 1},
		{KindUsage, 2},
		{KindLoanIncomplete, 2},
		{KindCancelled, 2},
		{KindConfirmationRequired, 2},
		{KindNotFound, 4},
		{KindIndexNotPublished, 4},
		{KindUpstreamUnavailable, 5},
		{KindRateLimited, 5},
		{KindNetwork, 6},
		{KindAPIChanged, 7},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.kind); got != tt.want {
			t.Errorf("ExitCode(%q) = %d, want %d", tt.kind, got, tt.want)
		}
	}
	if len(exitCodes) != len(tests) {
		t.Errorf("the table must cover exactly the %d named types, has %d", len(tests), len(exitCodes))
	}
}

// An unknown kind falls into internal (code 1): it never invents a new code,
// and the envelope says so in the message.
func TestUnknownKindFallsBackToInternalWithCode1(t *testing.T) {
	if got := ExitCode(Kind("mystery")); got != 1 {
		t.Errorf("ExitCode(unknown) = %d, want 1", got)
	}
	env, ok := From(New(Kind("mystery"), "algo falló", ""))
	if !ok {
		t.Fatal("From must recognise our own errors")
	}
	if env.Error.Type != string(KindInternal) {
		t.Errorf("unknown kind must be treated as internal, got type %q", env.Error.Type)
	}
	if env.Error.ExitCode != 1 {
		t.Errorf("unknown kind must use code 1, got %d", env.Error.ExitCode)
	}
	if !strings.Contains(env.Error.Message, "desconocido") {
		t.Errorf("the message must say the kind is unknown, got %q", env.Error.Message)
	}
	if !strings.Contains(env.Error.Message, "algo falló") {
		t.Errorf("the original message must survive, got %q", env.Error.Message)
	}
}

// The envelope has exactly {"error": {"type","message","exit_code","hint"}},
// and hint is omitted when empty. type and message are never missing.
func TestEnvelopeJSONShape(t *testing.T) {
	withHint, ok := From(New(KindIndexNotPublished, "El Euríbor a un año de 2026-12 aún no está publicado.", "usa --assume <valor> para un escenario"))
	if !ok {
		t.Fatal("From must recognise our own errors")
	}
	raw, err := json.Marshal(withHint)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("envelope is not JSON: %v (raw: %s)", err, raw)
	}
	if len(got) != 1 {
		t.Errorf("the envelope must have exactly one top-level key, got %v", got)
	}
	body, ok := got["error"]
	if !ok {
		t.Fatalf("the envelope must carry the error under \"error\", got %v", got)
	}
	wantKeys := map[string]bool{"type": true, "message": true, "exit_code": true, "hint": true}
	if len(body) != len(wantKeys) {
		t.Errorf("body keys = %v, want exactly %v", body, wantKeys)
	}
	for key := range wantKeys {
		if _, ok := body[key]; !ok {
			t.Errorf("body is missing key %q: %v", key, body)
		}
	}
	if body["type"] != string(KindIndexNotPublished) {
		t.Errorf("type = %v, want %q", body["type"], KindIndexNotPublished)
	}
	if body["exit_code"] != float64(4) {
		t.Errorf("exit_code = %v, want 4", body["exit_code"])
	}
	if body["message"] != "El Euríbor a un año de 2026-12 aún no está publicado." {
		t.Errorf("message = %v", body["message"])
	}
	if body["hint"] != "usa --assume <valor> para un escenario" {
		t.Errorf("hint = %v", body["hint"])
	}

	withoutHint, ok := From(New(KindUsage, "falta --amount", ""))
	if !ok {
		t.Fatal("From must recognise our own errors")
	}
	raw, err = json.Marshal(withoutHint)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("envelope is not JSON: %v (raw: %s)", err, raw)
	}
	body = got["error"]
	if _, ok := body["hint"]; ok {
		t.Errorf("an empty hint must be omitted from the JSON, got %v", body)
	}
	if len(body) != 3 {
		t.Errorf("body keys = %v, want exactly type, message and exit_code", body)
	}
	if body["exit_code"] != float64(2) {
		t.Errorf("exit_code = %v, want 2", body["exit_code"])
	}
}

// From recognises our own errors even when they are wrapped by intermediate
// layers, and refuses any foreign error.
func TestFromRecognisesOursAndRejectsForeign(t *testing.T) {
	if _, ok := From(errors.New("boom")); ok {
		t.Error("From must not claim a foreign error")
	}
	wrapped := fmt.Errorf("contexto: %w", New(KindNetwork, "sin red y sin caché", ""))
	env, ok := From(wrapped)
	if !ok {
		t.Fatal("From must unwrap and recognise our own errors")
	}
	if env.Error.Type != string(KindNetwork) || env.Error.ExitCode != 6 {
		t.Errorf("unexpected envelope: %+v", env)
	}
}
