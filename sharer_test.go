package upshare

import (
	"reflect"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func TestSharerDirective(t *testing.T) {
	type test struct {
		in  string
		out Sharer
	}

	var tests = []test{
		{
			in:  `* /tmp/shares`,
			out: Sharer{Symlink: "/tmp/shares"},
		},
		{
			in: `* {
				symlink /tmp/shares
				root /tmp/root
			}`,
			out: Sharer{
				Symlink: "/tmp/shares",
				Root:    "/tmp/root",
			},
		},
	}

	var h httpcaddyfile.Helper
	for i, test := range tests {
		h = h.WithDispenser(caddyfile.NewTestDispenser(test.in))

		v, err := parseSharerDirective(h)
		if err != nil {
			t.Errorf("error on %d: %v", i, err)
			continue
		}

		if !reflect.DeepEqual(&test.out, v) {
			t.Errorf("unexpected got %#v", v)
		}
	}
}
