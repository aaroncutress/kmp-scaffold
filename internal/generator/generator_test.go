package generator

import "testing"

func TestLayoutRegistry(t *testing.T) {
	if len(Layouts(KindAndroid)) < 2 {
		t.Error("expected at least two Android layouts to be registered")
	}
	if len(Layouts(KindIOS)) < 1 {
		t.Error("expected at least one iOS layout to be registered")
	}
	if _, err := Get(KindAndroid, "nav3-shell"); err != nil {
		t.Errorf("nav3-shell should be registered: %v", err)
	}
	if _, err := Get(KindIOS, "does-not-exist"); err == nil {
		t.Error("an unknown layout should be an error")
	}
}
