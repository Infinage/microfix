package spec

import "testing"

func Test_RouterLegacyCheck(t *testing.T) {
	t.Run("FIX4X - LegacyRouter", func(t *testing.T) {
		router, err := NewDefaultRouter("FIX40.xml")
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if !router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return true")
		}
	})

	t.Run("FIXT - LegacyRouter", func(t *testing.T) {
		router, err := NewRouter("FIXT11.xml", []string{"FIXT11.xml"})
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if !router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return true")
		}
	})

	t.Run("FIXT - Non LegacyRouter", func(t *testing.T) {
		router, err := NewRouter("FIXT11.xml", []string{"FIX40.xml"})
		if err != nil {
			t.Fatal("Failed to load router setup")
		} else if router.IsLegacyRouter() {
			t.Error("Expected IsLegacyRouter() to return false")
		}
	})
}
