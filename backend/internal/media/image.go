package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder (decode only; output is JPEG/PNG)
)

// Image size limits.
const (
	MaxPixelsPerSide = 12000
	MaxPixels        = 50_000_000
	MaxStoredSide    = 2560 // stored originals are downscaled to this longest side
	ThumbSide        = 400
	jpegQuality      = 85
)

// Detected MIME types.
const (
	MimeJPEG = "image/jpeg"
	MimePNG  = "image/png"
	MimeWebP = "image/webp"
)

// SniffMIME identifies jpeg/png/webp by magic bytes ONLY (file name and client headers are ignored).
func SniffMIME(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return MimeJPEG
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return MimePNG
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return MimeWebP
	}
	return ""
}

// ErrDimensions is returned for images whose pixel dimensions exceed the limits.
var ErrDimensions = errors.New("image dimensions too large")

// Processed is the result of Process.
type Processed struct {
	Data, Thumb []byte
	Mime        string // output MIME (image/jpeg or image/png)
	Ext         string // ".jpg" or ".png"
	Width       int
	Height      int
}

// Process decodes data (already MIME-sniffed), applies the EXIF orientation, downsizes huge images
// and re-encodes both the image and a 400px thumbnail. Re-encoding drops EVERY metadata block
// (EXIF/GPS, XMP, ICC, PNG text chunks). WebP input becomes JPEG (or PNG when it has transparency).
func Process(data []byte, mime string) (*Processed, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxPixelsPerSide || cfg.Height > MaxPixelsPerSide ||
		cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrDimensions
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	orientation := 1
	if mime == MimeJPEG {
		orientation = jpegOrientation(data)
	}
	img = fit(img, MaxStoredSide)
	img = orient(img, orientation)

	out := &Processed{Mime: MimeJPEG, Ext: ".jpg"}
	if mime == MimePNG || !isOpaque(img) {
		out.Mime, out.Ext = MimePNG, ".png"
	}
	out.Width, out.Height = img.Bounds().Dx(), img.Bounds().Dy()
	if out.Data, err = encode(img, out.Mime); err != nil {
		return nil, err
	}
	if out.Thumb, err = encode(fit(img, ThumbSide), out.Mime); err != nil {
		return nil, err
	}
	return out, nil
}

func encode(img image.Image, mime string) ([]byte, error) {
	var buf bytes.Buffer
	if mime == MimePNG {
		enc := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, err
		}
	} else if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return true
}

// fit downscales img so that its longest side is at most max (never upscales).
func fit(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	nw, nh := max, h*max/w
	if h > w {
		nw, nh = w*max/h, max
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// orient applies an EXIF orientation value (1..8) to the pixels so the stripped image still
// displays upright.
func orient(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if o >= 5 {
		nw, nh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)))
		}
	}
	return dst
}

// jpegOrientation reads the EXIF Orientation tag (0x0112) of a JPEG; 1 when absent or malformed.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end: no more metadata
			return 1
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 {
			i += 2
			continue
		}
		segLen := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if segLen < 2 || i+2+segLen > len(b) {
			return 1
		}
		if marker == 0xE1 {
			if o := exifOrientation(b[i+4 : i+2+segLen]); o != 0 {
				return o
			}
		}
		i += 2 + segLen
	}
	return 1
}

func exifOrientation(seg []byte) int {
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}
	t := seg[6:]
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	if bo.Uint16(t[2:4]) != 0x002A {
		return 0
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 {
			v := int(bo.Uint16(t[e+8 : e+10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}
