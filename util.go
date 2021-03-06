/*
 * Copyright (c) 2013 Kurt Jung (Gmail: kurt.w.jung)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package gofpdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	// "github.com/davecgh/go-spew/spew"

	"io"
	"math"
	"os"
)

func round(f float64) int {
	if f < 0 {
		return -int(math.Floor(-f + 0.5))
	}
	return int(math.Floor(f + 0.5))
}

func sprintf(fmtStr string, args ...interface{}) string {
	return fmt.Sprintf(fmtStr, args...)
}

// Returns true if the specified normal file exists
func fileExist(filename string) (ok bool) {
	info, err := os.Stat(filename)
	if err == nil {
		if ^os.ModePerm&info.Mode() == 0 {
			ok = true
		}
	}
	return ok
}

// Returns the size of the specified file; ok will be false
// if the file does not exist or is not an ordinary file
func fileSize(filename string) (size int64, ok bool) {
	info, err := os.Stat(filename)
	ok = err == nil
	if ok {
		size = info.Size()
	}
	return
}

// Returns a new buffer populated with the contents of the specified Reader
func bufferFromReader(r io.Reader) (b *bytes.Buffer, err error) {
	b = new(bytes.Buffer)
	_, err = b.ReadFrom(r)
	return
}

// Returns true if the two specified float slices are equal
func slicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Returns a zlib-compressed copy of the specified byte array
func sliceCompress(data []byte) []byte {
	var buf bytes.Buffer
	cmp, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	cmp.Write(data)
	cmp.Close()
	return buf.Bytes()
}

// Returns an uncompressed copy of the specified zlib-compressed byte array
func sliceUncompress(data []byte) (outData []byte, err error) {
	inBuf := bytes.NewBuffer(data)
	r, err := zlib.NewReader(inBuf)
	defer r.Close()
	if err == nil {
		var outBuf bytes.Buffer
		_, err = outBuf.ReadFrom(r)
		if err == nil {
			outData = outBuf.Bytes()
		}
	}
	return
}

// Convert UTF-8 to UTF-16BE with BOM; from http://www.fpdf.org/
func utf8toutf16(s string) string {
	res := make([]byte, 0, 8)
	res = append(res, 0xFE, 0xFF)
	nb := len(s)
	i := 0
	for i < nb {
		c1 := byte(s[i])
		i++
		if c1 >= 224 {
			// 3-byte character
			c2 := byte(s[i])
			i++
			c3 := byte(s[i])
			i++
			res = append(res, ((c1&0x0F)<<4)+((c2&0x3C)>>2),
				((c2&0x03)<<6)+(c3&0x3F))
		} else if c1 >= 192 {
			// 2-byte character
			c2 := byte(s[i])
			i++
			res = append(res, ((c1 & 0x1C) >> 2),
				((c1&0x03)<<6)+(c2&0x3F))
		} else {
			// Single-byte character
			res = append(res, 0, c1)
		}
	}
	return string(res)
}

// Return a if cnd is true, otherwise b
func intIf(cnd bool, a, b int) int {
	if cnd {
		return a
	}
	return b
}

// Return aStr if cnd is true, otherwise bStr
func strIf(cnd bool, aStr, bStr string) string {
	if cnd {
		return aStr
	}
	return bStr
}

// Transform moves a point by given X, Y offset
func (p *PointType) Transform(x, y float64) PointType {
	return PointType{p.X + x, p.Y + y}
}

// Orientation returns the orientation of a given size:
// "P" for portrait, "L" for landscape
func (s *SizeType) Orientation() string {
	if s == nil || s.Ht == s.Wd {
		return ""
	}
	if s.Wd > s.Ht {
		return "L"
	}
	return "P"
}

// ScaleBy expands a size by a certain factor
func (s *SizeType) ScaleBy(factor float64) SizeType {
	return SizeType{s.Wd * factor, s.Ht * factor}
}

// ScaleToWidth adjusts the height of a size to match the given width
func (s *SizeType) ScaleToWidth(width float64) SizeType {
	height := s.Ht * width / s.Wd
	return SizeType{width, height}
}

// ScaleToHeight adjsuts the width of a size to match the given height
func (s *SizeType) ScaleToHeight(height float64) SizeType {
	width := s.Wd * height / s.Ht
	return SizeType{width, height}
}
