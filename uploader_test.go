package upshare

import (
	"reflect"
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func TestUploaderDirective(t *testing.T) {
	type test struct {
		in  string
		out Uploader
	}

	var tests = []test{
		{
			in:  `* /tmp/shares`,
			out: Uploader{Root: "/tmp/shares"},
		},
		{
			in: `* {
				root /tmp/root
			}`,
			out: Uploader{Root: "/tmp/root"},
		},
	}

	var h httpcaddyfile.Helper
	for i, test := range tests {
		h = h.WithDispenser(caddyfile.NewTestDispenser(test.in))

		v, err := parseUploaderDirective(h)
		if err != nil {
			t.Errorf("error on %d: %v", i, err)
			continue
		}

		if !reflect.DeepEqual(&test.out, v) {
			t.Errorf("unexpected got %#v", v)
		}
	}
}
