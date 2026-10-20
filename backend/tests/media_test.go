package tests

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jpegWithEXIF builds a w x h JPEG carrying an EXIF APP1 segment with the given orientation and a
// fake GPS payload that must not survive re-encoding.
func jpegWithEXIF(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	var seg bytes.Buffer
	seg.WriteString("Exif\x00\x00")
	seg.WriteString("MM\x00\x2a\x00\x00\x00\x08")
	seg.Write([]byte{0x00, 0x01})                                     // one IFD entry
	seg.Write([]byte{0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01}) // tag 0x0112, SHORT, count 1
	seg.Write([]byte{0x00, byte(orientation), 0x00, 0x00})            // value
	seg.Write([]byte{0x00, 0x00, 0x00, 0x00})                         // next IFD
	seg.WriteString("GPSLatitude=41.38871 GPSLongitude=33.78274 SecretCamera")
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(seg.Len()+2))
	raw := buf.Bytes()
	out := append([]byte{}, raw[:2]...)
	out = append(out, app1...)
	out = append(out, seg.Bytes()...)
	return append(out, raw[2:]...)
}

func TestMediaUploadAndServe(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	other, _ := e.register("x@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)

	if r := e.upload("", id, "a.png", tinyPNG(8, 8, 1)); r.Status != 401 {
		t.Fatalf("anon upload: %d", r.Status)
	}
	r := e.upload(owner, id, "a.png", tinyPNG(8, 8, 1))
	if r.Status != 201 || r.data()["mime_type"] != "image/png" || r.data()["width"].(float64) != 8 {
		t.Fatalf("upload: %d %s", r.Status, r.Raw)
	}
	mid := r.data()["id"].(string)
	if r.data()["url"] != "/api/v1/media/"+mid || r.data()["thumb_url"] != "/api/v1/media/"+mid+"?thumb=1" {
		t.Fatalf("urls: %v", r.data())
	}
	// duplicate (same pixels -> same re-encoded bytes) is idempotent
	dup := e.upload(owner, id, "again.png", tinyPNG(8, 8, 1))
	if dup.Status != 200 || dup.data()["id"] != mid {
		t.Fatalf("duplicate upload: %d %s", dup.Status, dup.Raw)
	}
	var cnt int
	e.db.QueryRow(`SELECT count(*) FROM observation_media WHERE observation_id = ?`, id).Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("media rows: %d", cnt)
	}
	// detail shows media + thumbnail id
	d := e.get("/observations/"+id, owner).data()
	if len(d["media"].([]any)) != 1 || d["thumbnail_media_id"] != mid || d["media_count"].(float64) != 1 {
		t.Fatalf("detail media: %v", d)
	}
	// same visibility as the parent observation: hidden until verified
	for who, tok := range map[string]string{"anon": "", "other": other} {
		if g := e.get("/media/"+mid, tok); g.Status != 404 {
			t.Errorf("%s sees unverified media: %d", who, g.Status)
		}
	}
	if g := e.get("/media/"+mid, owner); g.Status != 200 || g.Header.Get("Content-Type") != "image/png" || g.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("owner media: %d %v", g.Status, g.Header)
	}
	if g := e.get("/media/"+mid+"?thumb=1", e.admin); g.Status != 200 {
		t.Fatalf("staff thumb: %d", g.Status)
	}
	e.decide(e.admin, id, "verify", nil)
	for who, tok := range map[string]string{"anon": "", "other": other} {
		if g := e.get("/media/"+mid, tok); g.Status != 200 || !bytes.HasPrefix(g.Raw, []byte("\x89PNG")) {
			t.Errorf("%s media after verify: %d", who, g.Status)
		}
	}
	// verified -> owner cannot add photos any more; non-owner is refused
	if r := e.upload(owner, id, "b.png", tinyPNG(8, 8, 2)); r.Status != 409 {
		t.Errorf("upload to verified: %d", r.Status)
	}
	if r := e.upload(other, id, "b.png", tinyPNG(8, 8, 2)); r.Status != 403 {
		t.Errorf("upload by non-owner of visible record: %d", r.Status)
	}
	// soft-deleted parent hides the image
	e.do("DELETE", "/observations/"+id, owner, nil)
	if g := e.get("/media/"+mid, ""); g.Status != 404 {
		t.Errorf("media of deleted observation: %d", g.Status)
	}
}

func TestMediaUploadValidation(t *testing.T) {
	e := newEnv(t) // MaxUploadMB = 2
	owner, _ := e.register("o@example.org")
	other, _ := e.register("x@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)

	// content is judged by magic bytes, never by extension or declared type
	if r := e.upload(owner, id, "photo.png", []byte("this is plain text, not an image")); r.Status != 415 || r.errCode() != "unsupported_media_type" {
		t.Errorf("text named .png: %d %s", r.Status, r.Raw)
	}
	if r := e.upload(owner, id, "x.jpg", []byte("%PDF-1.7 fake")); r.Status != 415 {
		t.Errorf("pdf: %d", r.Status)
	}
	if r := e.upload(owner, id, "x.png", []byte("GIF89a....")); r.Status != 415 {
		t.Errorf("gif: %d", r.Status)
	}
	if r := e.upload(owner, id, "x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`)); r.Status != 415 {
		t.Errorf("svg: %d", r.Status)
	}
	corrupt := tinyPNG(8, 8, 3)
	corrupt = corrupt[:len(corrupt)/2] // valid magic, truncated body
	if r := e.upload(owner, id, "c.png", corrupt); r.Status != 415 {
		t.Errorf("truncated png: %d %s", r.Status, r.Raw)
	}
	// oversize (limit 2 MB)
	big := append(tinyPNG(4, 4, 4), bytes.Repeat([]byte{0}, 3<<20)...)
	if r := e.upload(owner, id, "big.png", big); r.Status != 413 || r.errCode() != "payload_too_large" {
		t.Errorf("oversize: %d %s", r.Status, r.Raw)
	}
	// decompression bomb: tiny file, absurd dimensions
	if r := e.upload(owner, id, "bomb.png", hugePNGHeader()); r.Status != 413 && r.Status != 415 {
		t.Errorf("dimension bomb: %d", r.Status)
	}
	// no file field / not multipart
	if r := e.do("POST", "/observations/"+id+"/media", owner, map[string]any{"file": "x"}); r.Status != 400 {
		t.Errorf("json body: %d", r.Status)
	}
	// unknown / invisible observation
	if r := e.upload(owner, "00000000-0000-0000-0000-000000000000", "a.png", tinyPNG(4, 4, 1)); r.Status != 404 {
		t.Errorf("unknown observation: %d", r.Status)
	}
	if r := e.upload(other, id, "a.png", tinyPNG(4, 4, 1)); r.Status != 404 {
		t.Errorf("invisible observation: %d", r.Status)
	}
	// nothing was stored for the rejected files
	var cnt int
	e.db.QueryRow(`SELECT count(*) FROM observation_media`).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("rejected uploads left %d rows", cnt)
	}
	files := 0
	filepath.WalkDir(e.cfg.StorageDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files++
		}
		return nil
	})
	if files != 0 {
		t.Errorf("rejected uploads left %d files on disk", files)
	}
	// limit of 8 images
	for i := 0; i < 8; i++ {
		if r := e.upload(owner, id, "i.png", tinyPNG(6+i, 6, uint8(i*20))); r.Status != 201 {
			t.Fatalf("image %d: %d %s", i, r.Status, r.Raw)
		}
	}
	if r := e.upload(owner, id, "ninth.png", tinyPNG(20, 6, 99)); r.Status != 409 {
		t.Errorf("9th image: %d %s", r.Status, r.Raw)
	}
}

// hugePNGHeader is a syntactically valid PNG header claiming 60000x60000 pixels.
func hugePNGHeader() []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 60000)
	binary.BigEndian.PutUint32(ihdr[4:], 60000)
	ihdr[8], ihdr[9] = 8, 2
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.WriteString("IHDR")
	b.Write(ihdr)
	binary.Write(&b, binary.BigEndian, uint32(0)) // CRC not checked by DecodeConfig
	return b.Bytes()
}

func TestMediaStripsEXIFAndAppliesOrientation(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)
	src := jpegWithEXIF(t, 40, 20, 6) // rotate 90 CW -> 20x40
	if !bytes.Contains(src, []byte("GPSLatitude")) {
		t.Fatal("test fixture lacks the marker")
	}
	r := e.upload(owner, id, "../../evil name.txt", src) // hostile file name must not matter
	if r.Status != 201 || r.data()["mime_type"] != "image/jpeg" {
		t.Fatalf("upload: %d %s", r.Status, r.Raw)
	}
	if r.data()["width"].(float64) != 20 || r.data()["height"].(float64) != 40 {
		t.Fatalf("orientation not applied: %v", r.data())
	}
	g := e.get("/media/"+r.data()["id"].(string), owner)
	if g.Status != 200 || bytes.Contains(g.Raw, []byte("GPSLatitude")) || bytes.Contains(g.Raw, []byte("Exif")) || bytes.Contains(g.Raw, []byte("SecretCamera")) {
		t.Fatal("EXIF/GPS survived re-encoding")
	}
	img, err := jpeg.Decode(bytes.NewReader(g.Raw))
	if err != nil || img.Bounds().Dx() != 20 || img.Bounds().Dy() != 40 {
		t.Fatalf("served jpeg: %v %v", err, img)
	}
	// thumbnail exists and is a valid image; stored files have random names without the client name
	th := e.get("/media/"+r.data()["id"].(string)+"?thumb=1", owner)
	if _, err := jpeg.Decode(bytes.NewReader(th.Raw)); err != nil {
		t.Fatalf("thumb: %v", err)
	}
	var path string
	e.db.QueryRow(`SELECT storage_path FROM observation_media WHERE id = ?`, r.data()["id"]).Scan(&path)
	if strings.Contains(path, "evil") || strings.Contains(path, "..") || !strings.HasSuffix(path, ".jpg") {
		t.Fatalf("storage path %q", path)
	}
	if _, err := os.Stat(filepath.Join(e.cfg.StorageDir, path)); err != nil {
		t.Fatalf("file missing: %v", err)
	}
}

func TestMediaLargeImageIsDownscaledAndThumbnailed(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)
	r := e.upload(owner, id, "wide.png", tinyPNG(2600, 40, 5))
	if r.Status != 201 || r.data()["width"].(float64) != 2560 || r.data()["height"].(float64) != 39 {
		t.Fatalf("downscale: %d %s", r.Status, r.Raw)
	}
	th := e.get("/media/"+r.data()["id"].(string)+"?thumb=1", owner)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(th.Raw))
	if err != nil || cfg.Width != 400 || cfg.Height != 6 {
		t.Fatalf("thumb dims: %+v %v", cfg, err)
	}
}

func TestMediaWebP(t *testing.T) {
	// 1x1 lossless WebP
	webp, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)
	r := e.upload(owner, id, "a.webp", webp)
	if r.Status != 201 || (r.data()["mime_type"] != "image/jpeg" && r.data()["mime_type"] != "image/png") {
		t.Fatalf("webp: %d %s", r.Status, r.Raw)
	}
}

func TestMediaPathTraversalIsRefused(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	id := e.createObs(owner, merkezPt, nil)["id"].(string)
	r := e.upload(owner, id, "a.png", tinyPNG(8, 8, 1))
	mid := r.data()["id"].(string)
	for _, evil := range []string{"../../../../etc/passwd", "/etc/passwd", "2026/../../x.png", "a\x00b.png"} {
		e.db.Exec(`UPDATE observation_media SET storage_path = ? WHERE id = ?`, evil, mid)
		if g := e.get("/media/"+mid, owner); g.Status != 404 || bytes.Contains(g.Raw, []byte("root:")) {
			t.Errorf("path %q: %d", evil, g.Status)
		}
	}
}
