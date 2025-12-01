package client

import (
	"testing"
)

func TestAeroAPIVersion(t *testing.T) {
	if AeroAPIVersion != "4.28.0" {
		t.Errorf("AeroAPIVersion = %q, want %q", AeroAPIVersion, "4.28.0")
	}
}

func TestNewClient(t *testing.T) {
	c, err := NewClient("https://aeroapi.flightaware.com/aeroapi")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewClientWithResponses(t *testing.T) {
	c, err := NewClientWithResponses("https://aeroapi.flightaware.com/aeroapi")
	if err != nil {
		t.Fatalf("NewClientWithResponses() error = %v", err)
	}
	if c == nil {
		t.Fatal("NewClientWithResponses() returned nil")
	}
}
