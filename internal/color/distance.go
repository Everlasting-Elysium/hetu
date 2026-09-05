package color

import "math"

// Distance returns the CIEDE2000 color difference (ΔE00) between two Lab colors
// with unit weighting factors (kL=kC=kH=1). ΔE00 ≈ 1 is a just-noticeable
// difference; designers searching by color want a tolerance well above that.
// Implementation follows Sharma, Wu & Dalal (2005); verified against their
// published reference pairs in color_test.go.
func Distance(a, b Lab) float64 {
	const pow25_7 = 6103515625.0 // 25^7

	c1 := math.Hypot(a.A, a.B)
	c2 := math.Hypot(b.A, b.B)
	cBar := (c1 + c2) / 2
	cBar7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+pow25_7)))

	a1p, a2p := (1+g)*a.A, (1+g)*b.A
	c1p, c2p := math.Hypot(a1p, a.B), math.Hypot(a2p, b.B)
	h1p, h2p := hueDeg(a.B, a1p), hueDeg(b.B, a2p)

	dLp := b.L - a.L
	dCp := c2p - c1p
	dhp := deltaHue(h1p, h2p, c1p*c2p)
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(rad(dhp)/2)

	lBar := (a.L + b.L) / 2
	cBarp := (c1p + c2p) / 2
	hBarp := meanHue(h1p, h2p, c1p*c2p)

	t := 1 - 0.17*math.Cos(rad(hBarp-30)) + 0.24*math.Cos(rad(2*hBarp)) +
		0.32*math.Cos(rad(3*hBarp+6)) - 0.20*math.Cos(rad(4*hBarp-63))

	dTheta := 30 * math.Exp(-math.Pow((hBarp-275)/25, 2))
	cBarp7 := math.Pow(cBarp, 7)
	rc := 2 * math.Sqrt(cBarp7/(cBarp7+pow25_7))
	sl := 1 + (0.015*math.Pow(lBar-50, 2))/math.Sqrt(20+math.Pow(lBar-50, 2))
	sc := 1 + 0.045*cBarp
	sh := 1 + 0.015*cBarp*t
	rt := -math.Sin(rad(2*dTheta)) * rc

	lTerm := dLp / sl
	cTerm := dCp / sc
	hTerm := dHp / sh
	return math.Sqrt(lTerm*lTerm + cTerm*cTerm + hTerm*hTerm + rt*cTerm*hTerm)
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// hueDeg returns atan2(b, aPrime) normalized to [0,360); it is 0 for the neutral
// axis (both components zero) so achromatic colors compare consistently.
func hueDeg(b, aPrime float64) float64 {
	if b == 0 && aPrime == 0 {
		return 0
	}
	h := math.Atan2(b, aPrime) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

// deltaHue is the signed hue difference h2-h1 wrapped to (-180,180], or 0 when
// either color is achromatic (chroma product zero).
func deltaHue(h1, h2, chromaProduct float64) float64 {
	if chromaProduct == 0 {
		return 0
	}
	d := h2 - h1
	switch {
	case d > 180:
		return d - 360
	case d < -180:
		return d + 360
	default:
		return d
	}
}

// meanHue is the average hue, wrapping across the 0/360 boundary, or the plain
// sum when either color is achromatic.
func meanHue(h1, h2, chromaProduct float64) float64 {
	if chromaProduct == 0 {
		return h1 + h2
	}
	sum := h1 + h2
	if math.Abs(h1-h2) <= 180 {
		return sum / 2
	}
	if sum < 360 {
		return (sum + 360) / 2
	}
	return (sum - 360) / 2
}
