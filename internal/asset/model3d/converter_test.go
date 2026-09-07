package model3d

import "testing"

func TestNewConverter_Blender(t *testing.T) {
	conv, err := NewConverter("blender", "localhost:9090")
	if err != nil {
		t.Fatalf("NewConverter(blender) error = %v", err)
	}
	bc, ok := conv.(*BlenderConverter)
	if !ok {
		t.Fatalf("NewConverter(blender) = %T, want *BlenderConverter", conv)
	}
	if bc.addr != "localhost:9090" {
		t.Errorf("BlenderConverter.addr = %q, want localhost:9090", bc.addr)
	}
}

// TestNewConverter_Assimp branches on the host: assimp on PATH must yield an
// *AssimpConverter, and its absence must be a clear error rather than a nil.
func TestNewConverter_Assimp(t *testing.T) {
	conv, err := NewConverter("assimp", "")
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
	conv, err := NewConverter("", "")
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

// TestNewConverter_AutoFallsBackToBlender covers auto-detect choosing the
// Blender sidecar when assimp is unavailable but a sidecar addr is configured.
func TestNewConverter_AutoFallsBackToBlender(t *testing.T) {
	if assimpAvailable() {
		t.Skip("assimp on PATH; the blender auto-fallback path is not reachable here")
	}
	conv, err := NewConverter("", "localhost:9090")
	if err != nil {
		t.Fatalf("NewConverter(auto, blender) error = %v", err)
	}
	if _, ok := conv.(*BlenderConverter); !ok {
		t.Fatalf("NewConverter(auto, blender) = %T, want *BlenderConverter", conv)
	}
}

func TestNewConverter_InvalidBackend(t *testing.T) {
	conv, err := NewConverter("invalid", "")
	if err == nil {
		t.Fatalf("NewConverter(invalid) = %v, want error", conv)
	}
	if conv != nil {
		t.Errorf("NewConverter(invalid) converter = %v, want nil on error", conv)
	}
}
