package image

// proTool is one external converter candidate. When stdout is true the sRGB
// raster is read from the process's stdout; otherwise the tool writes it to
// workDir/out+outExt, which the caller decodes. args builds the argv (never via
// a shell) from the input path, a scratch workDir, and the desired output path.
type proTool struct {
	bin    string
	stdout bool
	outExt string
	args   func(in, workDir, out string) []string
}

// proToolsByFormat lists converters per format in preference order (best color
// fidelity first, most-available fallback last). decode tries them in turn until
// one yields a decodable raster, so a tool that rejects a file (e.g. a wrong
// extension hint) transparently falls back to the next.
var proToolsByFormat = map[proFormat][]proTool{
	fmtRAW: {
		{bin: "darktable-cli", outExt: ".png", args: darktableArgs},
		{bin: "dcraw", stdout: true, args: dcrawArgs},
		{bin: "magick", stdout: true, args: magickSRGBArgs},
		{bin: "convert", stdout: true, args: magickSRGBArgs},
	},
	fmtHEIC: {
		{bin: "heif-dec", outExt: ".png", args: heifArgs},
		{bin: "heif-convert", outExt: ".png", args: heifArgs},
		{bin: "magick", stdout: true, args: magickSRGBArgs},
		{bin: "convert", stdout: true, args: magickSRGBArgs},
	},
	fmtEXR: {
		{bin: "oiiotool", outExt: ".png", args: oiiotoolArgs},
		{bin: "ffmpeg", stdout: true, args: ffmpegEXRArgs},
		{bin: "magick", stdout: true, args: magickEXRArgs},
		{bin: "convert", stdout: true, args: magickEXRArgs},
	},
}

// darktable-cli <in> <out> --core ...: pin config/cache to the scratch dir so it
// never touches $HOME, and disable OpenCL for deterministic headless runs. The
// scratch dir is empty, so darktable's refusal to overwrite is a non-issue.
func darktableArgs(in, workDir, out string) []string {
	return []string{in, out, "--core", "--configdir", workDir, "--cachedir", workDir, "--disable-opencl"}
}

// dcraw -c -w -T: write to stdout (-c), apply the camera white balance (-w), and
// emit TIFF (-T) which the imaging library can decode.
func dcrawArgs(in, _, _ string) []string {
	return []string{"-c", "-w", "-T", in}
}

// heif-dec / heif-convert <in> <out.png>: libheif detects the codec from content.
func heifArgs(in, _, out string) []string {
	return []string{in, out}
}

// oiiotool ... --colorconvert linear sRGB: OpenImageIO understands EXR's
// scene-linear data and applies the correct linear→sRGB transfer in one step.
func oiiotoolArgs(in, _, out string) []string {
	return []string{in, "--colorconvert", "linear", "sRGB", "-o", out}
}

// ffmpegEXRArgs streams a single PNG frame to stdout. The OpenEXR decoder emits
// scene-linear data; the eq filter's gamma applies pow(in, 1/2.2), an
// approximate linear→display transfer that keeps HDR previews from looking dark.
func ffmpegEXRArgs(in, _, _ string) []string {
	return []string{"-nostdin", "-i", in, "-vf", "eq=gamma=2.2", "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "pipe:1"}
}

// magickSRGBArgs converts any input ImageMagick can read to an sRGB PNG on
// stdout, honoring the embedded ICC profile during the colorspace transform.
func magickSRGBArgs(in, _, _ string) []string {
	return []string{in, "-colorspace", "sRGB", "png:-"}
}

// magickEXRArgs adds an -auto-level exposure safeguard for HDR EXR previews on
// builds where values above 1.0 would otherwise clip to white.
func magickEXRArgs(in, _, _ string) []string {
	return []string{in, "-colorspace", "sRGB", "-depth", "8", "-auto-level", "png:-"}
}
