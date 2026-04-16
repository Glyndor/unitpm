package env

import "testing"

func TestInt(t *testing.T) {
	t.Setenv("LYNX_TEST_INT", "42")
	if v := Int("LYNX_TEST_INT", 99); v != 42 {
		t.Errorf("got %d want 42", v)
	}
	if v := Int("LYNX_TEST_INT_MISSING", 99); v != 99 {
		t.Errorf("got %d want 99", v)
	}
	t.Setenv("LYNX_TEST_INT_BAD", "nope")
	if v := Int("LYNX_TEST_INT_BAD", 99); v != 99 {
		t.Errorf("got %d want fallback on bad int", v)
	}
	t.Setenv("LYNX_TEST_INT_ZERO", "0")
	if v := Int("LYNX_TEST_INT_ZERO", 99); v != 99 {
		t.Errorf("0 is not positive, expected fallback, got %d", v)
	}
	t.Setenv("LYNX_TEST_INT_NEG", "-5")
	if v := Int("LYNX_TEST_INT_NEG", 99); v != 99 {
		t.Errorf("negative is not positive, expected fallback, got %d", v)
	}
}

func TestInt64(t *testing.T) {
	t.Setenv("LYNX_TEST_I64", "1073741824") // 1 GiB
	if v := Int64("LYNX_TEST_I64", 0); v != 1073741824 {
		t.Errorf("got %d", v)
	}
	if v := Int64("LYNX_TEST_I64_MISS", 123); v != 123 {
		t.Errorf("fallback: got %d", v)
	}
}
