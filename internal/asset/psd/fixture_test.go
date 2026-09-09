package psd

import (
	"bytes"
	"encoding/binary"
)

// This file assembles valid Photoshop (PSD, version 1) byte streams for tests,
// so the handler is exercised without shipping a binary fixture. PSD is
// big-endian; the layout follows the Adobe spec:
// https://www.adobe.com/devnet-apps/photoshop/fileformatashtml/

// flatPSD builds a valid flattened 8-bit RGB PSD (no layers) with a solid-color
// merged image, exercising Extract (dimensions), Thumbnail (the merged image),
// and metadata (color mode, depth, layer_count=0).
func flatPSD(width, height int) []byte {
	var b bytes.Buffer
	writeHeader(&b, width, height)
	be32(&b, 0) // color mode data length
	be32(&b, 0) // image resources length
	be32(&b, 0) // layer and mask info length (no layers)
	writeMergedImage(&b, width, height)
	return b.Bytes()
}

// layeredPSD builds a valid 8-bit RGB PSD carrying numLayers empty (0×0) layers
// plus a merged image, so ExtractMetadata reports layer_count == numLayers.
func layeredPSD(width, height, numLayers int) []byte {
	var b bytes.Buffer
	writeHeader(&b, width, height)
	be32(&b, 0) // color mode data length
	be32(&b, 0) // image resources length

	li := layerInfo(numLayers)  // layer count + records + channel image data
	be32(&b, uint32(4+len(li))) // layer and mask info length (layer-info field + payload)
	be32(&b, uint32(len(li)))   // layer info length
	b.Write(li)

	writeMergedImage(&b, width, height)
	return b.Bytes()
}

// psdNoMergedImage builds a header with empty sections and NO image data
// section, so psd.Decode fails reading the merged image and the handler returns
// domain.ErrNoThumbnail.
func psdNoMergedImage(width, height int) []byte {
	var b bytes.Buffer
	writeHeader(&b, width, height)
	be32(&b, 0) // color mode data length
	be32(&b, 0) // image resources length
	be32(&b, 0) // layer and mask info length
	return b.Bytes()
}

func writeHeader(b *bytes.Buffer, width, height int) {
	b.WriteString("8BPS")
	be16(b, 1)               // version 1 (PSD)
	b.Write(make([]byte, 6)) // reserved
	be16(b, 3)               // channels: RGB
	be32(b, uint32(height))
	be32(b, uint32(width))
	be16(b, 8) // depth (bits per channel)
	be16(b, 3) // color mode: RGB
}

// writeMergedImage writes the image data section: a raw (uncompressed) 8-bit
// planar RGB buffer filled with a solid color, so the decoded thumbnail is a
// real image.
func writeMergedImage(b *bytes.Buffer, width, height int) {
	be16(b, 0) // compression: raw
	plane := width * height
	for _, v := range []byte{0xE0, 0x90, 0x30} { // R, G, B planes (solid orange)
		b.Write(bytes.Repeat([]byte{v}, plane))
	}
}

// layerInfo builds the Layer Info payload: the 2-byte layer count, one record
// per layer, then per-layer channel image data.
func layerInfo(numLayers int) []byte {
	var b bytes.Buffer
	be16(&b, uint16(numLayers))
	for range numLayers {
		b.Write(layerRecord())
	}
	// One channel per layer, each a bare 2-byte raw compression marker (the
	// layers are 0×0, so there are no pixel bytes to follow it).
	for range numLayers {
		be16(&b, 0)
	}
	return b.Bytes()
}

// layerRecord builds one 0×0, single-channel, unnamed layer record.
func layerRecord() []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 16))   // rect: top/left/bottom/right = 0 (0×0)
	be16(&b, 1)                 // channel count
	be16(&b, 0)                 // channel id 0 (red)
	be32(&b, 2)                 // channel data length (2 = just the compression marker)
	b.WriteString("8BIM")       // blend mode signature
	b.WriteString("norm")       // blend mode key (normal)
	b.WriteByte(255)            // opacity
	b.WriteByte(0)              // clipping
	b.WriteByte(0)              // flags
	b.WriteByte(0)              // filler
	be32(&b, 12)                // extra data length: mask(4) + blending ranges(4) + name(4)
	be32(&b, 0)                 // layer mask data length
	be32(&b, 0)                 // layer blending ranges length
	b.Write([]byte{0, 0, 0, 0}) // Pascal name: length 0, padded to 4 bytes
	return b.Bytes()
}

func be16(b *bytes.Buffer, v uint16) { _ = binary.Write(b, binary.BigEndian, v) }
func be32(b *bytes.Buffer, v uint32) { _ = binary.Write(b, binary.BigEndian, v) }
