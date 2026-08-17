package capreg

import "testing"

func TestClampTimeout(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, defaultTimeoutMS},
		{-5, defaultTimeoutMS},
		{500, 500},
		{999999, maxTimeoutMS},
	}
	for _, c := range cases {
		if got := clampTimeout(c.in); got != c.want {
			t.Errorf("clampTimeout(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampReadBytes(t *testing.T) {
	if clampReadBytes(0) != defaultReadBytes {
		t.Errorf("clampReadBytes(0) = %d, want %d", clampReadBytes(0), defaultReadBytes)
	}
	if clampReadBytes(-1) != defaultReadBytes {
		t.Errorf("clampReadBytes(-1) = %d, want %d", clampReadBytes(-1), defaultReadBytes)
	}
	if clampReadBytes(1<<20) != maxReadBytes {
		t.Errorf("clampReadBytes(big) = %d, want %d", clampReadBytes(1<<20), maxReadBytes)
	}
	if clampReadBytes(128) != 128 {
		t.Errorf("clampReadBytes(128) = %d, want 128", clampReadBytes(128))
	}
}
