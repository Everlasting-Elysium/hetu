package model3d

import "testing"

// TestNewConverter_Assimp branches on the host: assimp on PATH must yield an
// *AssimpConverter, and its absence must be a clear error rather than a nil.
func TestNewConverter_Assimp(t *testing.T) {
	conv, err := NewConverter("assimp")
	if assimpAvailable() {
		if err != nil {
			t.Fatalf("NewConverter(assimp) error = %v, want success (assimp on PATH)", err)
		}
		if _, ok := conv.(*AssimpConverter); !ok {
			t.Fatalf("NewConverter(assimp) = %T, want *AssimpConverter", conv)
		}
		return
	}
	if err == nil {
		t.Fatalf("NewConverter(assimp) = %v, want error when assimp not on PATH", conv)
	}
}

// TestNewConverter_AutoDetect verifies empty backend auto-selects assimp when
// present, else returns a nil converter so conversion degrades gracefully.
func TestNewConverter_AutoDetect(t *testing.T) {
	conv, err := NewConverter("")
	if err != nil {
		t.Fatalf("NewConverter(auto) error = %v", err)
	}
	if assimpAvailable() {
		if _, ok := conv.(*AssimpConverter); !ok {
			t.Fatalf("NewConverter(auto) = %T, want *AssimpConverter", conv)
		}
		return
	}
	if conv != nil {
		t.Fatalf("NewConverter(auto) = %v, want nil when no backend available", conv)
	}
}

func TestNewConverter_InvalidBackend(t *testing.T) {
	conv, err := NewConverter("invalid")
	if err == nil {
		t.Fatalf("NewConverter(invalid) = %v, want error", conv)
	}
	if conv != nil {
		t.Errorf("NewConverter(invalid) converter = %v, want nil on error", conv)
	}
}
