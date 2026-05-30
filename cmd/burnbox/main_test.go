package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDispatch(t *testing.T) {
	cases := []struct {
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{nil, 2, "", "usage:"},
		{[]string{"version"}, 0, "burnbox ", ""},
		{[]string{"help"}, 0, "usage:", ""},
		{[]string{"--help"}, 0, "usage:", ""},
		{[]string{"bogus"}, 2, "", "unknown command"},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		code := run(c.args, &out, &errb)
		if code != c.wantCode {
			t.Errorf("run(%v) code = %d, want %d", c.args, code, c.wantCode)
		}
		if c.wantOut != "" && !strings.Contains(out.String(), c.wantOut) {
			t.Errorf("run(%v) stdout = %q, want substring %q", c.args, out.String(), c.wantOut)
		}
		if c.wantErr != "" && !strings.Contains(errb.String(), c.wantErr) {
			t.Errorf("run(%v) stderr = %q, want substring %q", c.args, errb.String(), c.wantErr)
		}
	}
}

func TestServeBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"serve", "-nope"}, &out, &errb); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}
