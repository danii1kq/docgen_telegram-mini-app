package service

import "testing"

// TestQrUploadAcceptsOnlyRasterImages checks the QR upload validation: the file
// type is sniffed by content and only raster images are stored. An SVG can
// carry a script and would run in the API's own origin when the QR link is
// opened directly - so it is not accepted anymore.
func TestQrUploadAcceptsOnlyRasterImages(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if ext, err := imageExt(png); err != nil || ext != ".png" {
		t.Fatalf("png: ext = %q, err = %v", ext, err)
	}

	jpeg := append([]byte("\xff\xd8\xff\xe0"), make([]byte, 64)...)
	if ext, err := imageExt(jpeg); err != nil || ext != ".jpg" {
		t.Fatalf("jpeg: ext = %q, err = %v", ext, err)
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if _, err := imageExt(svg); err == nil {
		t.Fatal("an SVG upload must be rejected")
	}

	html := []byte(`<html><script>alert(1)</script></html>`)
	if _, err := imageExt(html); err == nil {
		t.Fatal("an HTML upload must be rejected")
	}
}
