package image

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// jpegSegments holds raw IPTC and XMP payloads extracted from JPEG APPn markers.
type jpegSegments struct {
	iptc []byte // APP13 IPTC-IIM payload (after Photoshop 3.0 header)
	xmp  []byte // APP1 XMP payload (raw XML)
}

// scanJPEGSegments reads JPEG APPn markers to extract IPTC (APP13) and XMP
// (APP1) payloads. Non-JPEG files or read errors return empty segments.
func scanJPEGSegments(r io.Reader) (jpegSegments, error) {
	var seg jpegSegments
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return seg, err
	}
	if buf[0] != 0xFF || buf[1] != 0xD8 {
		return seg, nil // not a JPEG
	}
	for {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return seg, nil
		}
		if buf[0] != 0xFF {
			return seg, nil
		}
		marker := buf[1]
		// SOS (0xDA) begins the compressed image data; stop scanning.
		if marker == 0xDA {
			return seg, nil
		}
		// Skip standalone markers (RST, SOI, EOI, TEM).
		if marker == 0x00 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9) {
			continue
		}
		var lenBuf [2]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return seg, nil
		}
		segLen := int(binary.BigEndian.Uint16(lenBuf[:])) - 2
		if segLen < 0 {
			return seg, nil
		}
		data := make([]byte, segLen)
		if _, err := io.ReadFull(r, data); err != nil {
			return seg, nil
		}
		switch marker {
		case 0xE1: // APP1 — may be XMP
			if seg.xmp == nil && bytes.HasPrefix(data, xmpPrefix) {
				seg.xmp = data[len(xmpPrefix):]
			}
		case 0xED: // APP13 — may be IPTC (Photoshop IRB)
			if seg.iptc == nil && bytes.HasPrefix(data, photoshopPrefix) {
				seg.iptc = data[len(photoshopPrefix):]
			}
		}
	}
}

var (
	xmpPrefix       = append([]byte("http://ns.adobe.com/xap/1.0/"), 0x00)
	photoshopPrefix = append([]byte("Photoshop 3.0"), 0x00)
)

// iptcRecordTag identifies an IPTC-IIM dataset.
const (
	iptcRecord2  = 2
	iptcKeywords = 25
)

// extractIPTC parses IPTC-IIM keywords from a Photoshop IRB payload.
func extractIPTC(irb []byte, md *domain.ExtractedMetadata) {
	iptcData := findIPTCBlock(irb)
	if iptcData == nil {
		return
	}
	var keywords []string
	for len(iptcData) >= 5 {
		if iptcData[0] != 0x1C {
			break
		}
		rec := iptcData[1]
		tag := iptcData[2]
		dLen := int(binary.BigEndian.Uint16(iptcData[3:5]))
		iptcData = iptcData[5:]
		if len(iptcData) < dLen {
			break
		}
		if rec == iptcRecord2 && tag == iptcKeywords {
			kw := strings.TrimSpace(string(iptcData[:dLen]))
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
		iptcData = iptcData[dLen:]
	}
	if len(keywords) > 0 {
		md.Annotations[domain.KeyIPTCKeywords] = keywords
		md.Keywords = append(md.Keywords, keywords...)
	}
}

// findIPTCBlock locates the IPTC-IIM block (resource ID 0x0404) inside a
// Photoshop IRB (Image Resource Block) payload.
func findIPTCBlock(irb []byte) []byte {
	for len(irb) >= 12 {
		if !bytes.HasPrefix(irb, []byte("8BIM")) {
			irb = irb[1:] // try to resync
			continue
		}
		resID := binary.BigEndian.Uint16(irb[4:6])
		// Pascal string: first byte is length, then padded to even.
		pLen := int(irb[6])
		pPadded := pLen + 1
		if pPadded%2 != 0 {
			pPadded++
		}
		hdr := 4 + 2 + 1 + pPadded // "8BIM" + resID + pascal
		if len(irb) < hdr+4 {
			return nil
		}
		dataLen := int(binary.BigEndian.Uint32(irb[hdr : hdr+4]))
		start := hdr + 4
		if len(irb) < start+dataLen {
			return nil
		}
		if resID == 0x0404 {
			return irb[start : start+dataLen]
		}
		// Advance past this resource; data is padded to even length.
		advance := start + dataLen
		if advance%2 != 0 {
			advance++
		}
		irb = irb[advance:]
	}
	return nil
}

// extractXMP parses XMP XML for common Dublin Core and XMP fields.
func extractXMP(data []byte, md *domain.ExtractedMetadata) {
	var rdf xmpRDF
	if err := xml.Unmarshal(data, &rdf); err != nil {
		return
	}
	for _, desc := range rdf.Descriptions {
		if s := strings.TrimSpace(desc.Creator); s != "" {
			md.Annotations[domain.KeyXMPCreator] = s
		}
		if s := strings.TrimSpace(desc.Copyright); s != "" {
			md.Annotations[domain.KeyXMPCopyright] = s
		}
		if s := strings.TrimSpace(desc.Description.Value); s != "" {
			md.Annotations[domain.KeyXMPDescription] = s
		}
		subjects := collectBag(desc.Subject)
		if len(subjects) > 0 {
			md.Annotations[domain.KeyXMPSubject] = subjects
			md.Keywords = append(md.Keywords, subjects...)
		}
	}
}

// XMP RDF structures — minimal subset for Dublin Core fields.
type xmpRDF struct {
	XMLName      xml.Name       `xml:"RDF"`
	Descriptions []xmpDescBlock `xml:"Description"`
}

type xmpDescBlock struct {
	Creator     string     `xml:"creator"` // simple string form
	Copyright   string     `xml:"rights"`
	Description xmpAltVal  `xml:"description"`
	Subject     xmpBagNode `xml:"subject"`
}

type xmpAltVal struct {
	Value string `xml:",chardata"`
	Items []struct {
		Value string `xml:",chardata"`
	} `xml:"Alt>li"`
}

type xmpBagNode struct {
	Items []struct {
		Value string `xml:",chardata"`
	} `xml:"Bag>li"`
}

func collectBag(bag xmpBagNode) []string {
	var out []string
	for _, item := range bag.Items {
		s := strings.TrimSpace(item.Value)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
