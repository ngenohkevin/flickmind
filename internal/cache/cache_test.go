package cache

import (
	"testing"
	"time"
)

func TestSetAndGet_Roundtrip(t *testing.T) {
	c := New("") // in-memory

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	c.Set("test:key", payload{Name: "hello", Count: 42}, 1*time.Hour)

	var got payload
	if !c.Get("test:key", &got) {
		t.Fatal("expected key to exist")
	}
	if got.Name != "hello" {
		t.Errorf("expected name hello, got %s", got.Name)
	}
	if got.Count != 42 {
		t.Errorf("expected count 42, got %d", got.Count)
	}
}

func TestGet_Expired(t *testing.T) {
	c := New("") // in-memory

	c.Set("expire:key", "data", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	var got string
	if c.Get("expire:key", &got) {
		t.Error("expected expired key to return false")
	}
}

func TestGet_MissingKey(t *testing.T) {
	c := New("")

	var got string
	if c.Get("nonexistent", &got) {
		t.Error("expected missing key to return false")
	}
}

func TestInvalidatePrefix(t *testing.T) {
	c := New("")

	c.Set("user1:catalog:a", "data1", 1*time.Hour)
	c.Set("user1:catalog:b", "data2", 1*time.Hour)
	c.Set("user2:catalog:a", "data3", 1*time.Hour)

	c.InvalidatePrefix("user1:")

	var got string
	if c.Get("user1:catalog:a", &got) {
		t.Error("user1:catalog:a should be invalidated")
	}
	if c.Get("user1:catalog:b", &got) {
		t.Error("user1:catalog:b should be invalidated")
	}
	if !c.Get("user2:catalog:a", &got) {
		t.Error("user2:catalog:a should still exist")
	}
}

func TestSet_OverwritesExisting(t *testing.T) {
	c := New("")

	c.Set("key", "first", 1*time.Hour)
	c.Set("key", "second", 1*time.Hour)

	var got string
	if !c.Get("key", &got) {
		t.Fatal("expected key to exist")
	}
	if got != "second" {
		t.Errorf("expected second, got %s", got)
	}
}
