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

// Utility to generate font definition files

// Version: 1.2
// Date:    2011-06-18
// Author:  Olivier PLATHEY
// Port to Go: Kurt Jung, 2013-07-15

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
)

// AddFont imports a TrueType or OpenType font and makes it available.
// It is not necessary to call this function for the core PDF fonts
// (courier, helvetica, times, zapfdingbats).
//
// If it is not found, the error "Could not include font definition file" is set.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// fileStr specifies the base name with ".ttf/otf" extension of the font
// definition file to be added. The file will be loaded from the font directory
// specified in the call to New() or SetFontLocation().
func (f *Fpdf) AddFont(familyStr, styleStr, fileStr string) {
	fontkey := getFontKey(familyStr, styleStr)
	if _, ok := f.fonts[fontkey]; ok {
		return
	}
	FileStr := fileStr
	if FileStr == "" {
		FileStr = strings.Replace(familyStr, " ", "", -1) + strings.ToLower(styleStr) + ".ttf"
	}
	fullFileStr := path.Join(f.fontpath, FileStr)
	abort := func() {
		fmt.Println("Failed to AddTTFFont, aborting to AddFont")
		f.AddFont(familyStr, styleStr, fileStr)
	}

	// load the friggen font
	encList, err := loadMap(path.Join(f.fontpath, "cp1252.map"))
	if err != nil {
		abort()
		return
	}
	info, err := getInfoFromTrueType(fullFileStr, os.Stdout, true, encList)
	if err != nil {
		abort()
		return
	}
	info.Tp = "TrueType"

	info.I = len(f.fonts)
	// dbg("font [%s], type [%s]", info.File, info.Tp)
	f.fonts[fontkey] = info
}

// getFontKey is used by AddFontFromReader and GetFontDesc
func getFontKey(familyStr, styleStr string) string {
	familyStr = strings.ToLower(familyStr)
	styleStr = strings.ToUpper(styleStr)
	if styleStr == "IB" {
		styleStr = "BI"
	}
	return familyStr + styleStr
}

// GetFontDesc returns the font descriptor, which can be used for
// example to find the baseline of a font. If familyStr is empty
// current font descriptor will be returned.
// See FontDescType for documentation about the font descriptor.
// See AddFont for details about familyStr and styleStr.
func (f *Fpdf) GetFontDesc(familyStr, styleStr string) FontDescType {
	if familyStr == "" {
		return f.currentFont.Desc
	}
	return f.fonts[getFontKey(familyStr, styleStr)].Desc
}

// SetFont sets the font used to print character strings. It is mandatory to
// call this method at least once before printing text or the resulting
// document will not be valid.
//
// The font can be either a standard one or a font added via the AddFont()
// method or AddFontFromReader() method. Standard fonts use the Windows
// encoding cp1252 (Western Europe).
//
// The method can be called before the first page is created and the font is
// kept from page to page. If you just wish to change the current font size, it
// is simpler to call SetFontSize().
//
// Note: the font definition file must be accessible. An error is set if the
// file cannot be read.
//
// familyStr specifies the font family. It can be either a name defined by
// AddFont(), AddFontFromReader() or one of the standard families (case
// insensitive): "Courier" for fixed-width, "Helvetica" or "Arial" for sans
// serif, "Times" for serif, "Symbol" or "ZapfDingbats" for symbolic.
//
// styleStr can be "B" (bold), "I" (italic), "U" (underscore) or any
// combination. The default value (specified with an empty string) is regular.
// Bold and italic styles do not apply to Symbol and ZapfDingbats.
//
// size is the font size measured in points. The default value is the current
// size. If no size has been specified since the beginning of the document, the
// value taken is 12.
func (f *Fpdf) SetFont(familyStr, styleStr string, size float64) {
	// dbg("SetFont x %.2f, lMargin %.2f", f.x, f.lMargin)
	if f.err != nil {
		return
	}
	// dbg("SetFont")
	if familyStr == "" {
		familyStr = f.fontFamily
	} else {
		familyStr = strings.ToLower(familyStr)
	}
	styleStr = strings.ToUpper(styleStr)
	f.underline = strings.Contains(styleStr, "U")
	if f.underline {
		styleStr = strings.Replace(styleStr, "U", "", -1)
	}
	if styleStr == "IB" {
		styleStr = "BI"
	}
	if size == 0.0 {
		size = f.fontSizePt
	}
	// Test if font is already selected
	if f.fontFamily == familyStr && f.fontStyle == styleStr && f.fontSizePt == size {
		return
	}
	// Test if font is already loaded
	fontkey := familyStr + styleStr
	if _, ok := f.fonts[fontkey]; !ok {
		f.err = fmt.Errorf("undefined font: %s %s", familyStr, styleStr)
		return
	}
	// Select it
	f.fontFamily = familyStr
	f.fontStyle = styleStr
	f.fontSizePt = size
	f.fontSize = size / f.k
	f.currentFont = f.fonts[fontkey]
	if f.page > 0 {
		f.outf("BT /F%d %.2f Tf ET", f.currentFont.I, f.fontSizePt)
	}
	return
}

func (f *Fpdf) putfonts() {
	if f.err != nil {
		return
	}
	{
		var fileList []string
		lookup := make(map[string]fontType)
		for _, info := range f.fonts {
			if len(info.Data) > 0 {
				fileList = append(fileList, info.Name)
				lookup[info.Name] = info
			}
		}
		if f.catalogSort {
			sort.Strings(fileList)
		}
		for _, fontFile := range fileList {
			info := lookup[fontFile]
			// Font file embedding
			f.newobj()
			info.N = f.n
			// dbg("font file [%s], ext [%s]", file, file[len(file)-2:])
			f.outf("<</Length %d", len(info.Data))
			f.out("/Filter /FlateDecode") // zlib compressed ttf
			f.outf("/Length1 %d", info.OrigLen)
			f.out(">>")
			f.putstream(info.Data)
			f.out("endobj")
		}
	}
	{
		var keyList []string
		var font fontType
		var key string
		for key = range f.fonts {
			keyList = append(keyList, key)
		}
		if f.catalogSort {
			sort.Strings(keyList)
		}
		for _, key = range keyList {
			font = f.fonts[key]
			// Font objects
			origN := f.n
			font.N = f.n + 1
			f.fonts[key] = font
			name := font.Name
			if font.Tp != "TrueType" {
				f.err = fmt.Errorf("unsupported font type: %s", font.Tp)
				return
			}

			// Additional Type1 or TrueType/OpenType font
			f.newobj()
			f.out("<</Type /Font")
			f.outf("/BaseFont /%s", name)
			f.outf("/Subtype /%s", font.Tp)
			f.out("/FirstChar 32 /LastChar 255")
			f.outf("/Widths %d 0 R", f.n+1)
			f.outf("/FontDescriptor %d 0 R", f.n+2)
			f.out("/Encoding /WinAnsiEncoding") // test...
			f.out(">>")
			f.out("endobj")
			// Widths
			f.newobj()
			var s fmtBuffer
			s.WriteString("[")
			for j := 32; j < 256; j++ {
				s.printf("%d ", font.Cw[j])
			}
			s.WriteString("]")
			f.out(s.String())
			f.out("endobj")
			// Descriptor
			f.newobj()
			s.Truncate(0)
			s.printf("<</Type /FontDescriptor /FontName /%s ", name)
			s.printf("/Ascent %d ", font.Desc.Ascent)
			s.printf("/Descent %d ", font.Desc.Descent)
			s.printf("/CapHeight %d ", font.Desc.CapHeight)
			s.printf("/Flags %d ", font.Desc.Flags)
			s.printf("/FontBBox [%d %d %d %d] ", font.Desc.FontBBox.Xmin, font.Desc.FontBBox.Ymin,
				font.Desc.FontBBox.Xmax, font.Desc.FontBBox.Ymax)
			s.printf("/ItalicAngle %d ", font.Desc.ItalicAngle)
			s.printf("/MissingWidth %d ", font.Desc.MissingWidth)
			s.printf("/FontFile2 %d 0 R>>", origN)
			f.out(s.String())
			f.out("endobj")
		}
	}
}

func loadMap(encodingFileStr string) (encList encListType, err error) {
	// printf("Encoding file string [%s]\n", encodingFileStr)
	var f *os.File
	// f, err = os.Open(encodingFilepath(encodingFileStr))
	f, err = os.Open(encodingFileStr)
	if err == nil {
		defer f.Close()
		for j := range encList {
			encList[j].uv = -1
			encList[j].name = ".notdef"
		}
		scanner := bufio.NewScanner(f)
		var enc encType
		var pos int
		for scanner.Scan() {
			// "!3F U+003F question"
			_, err = fmt.Sscanf(scanner.Text(), "!%x U+%x %s", &pos, &enc.uv, &enc.name)
			if err == nil {
				if pos < 256 {
					encList[pos] = enc
				} else {
					err = fmt.Errorf("map position 0x%2X exceeds 0xFF", pos)
					return
				}
			} else {
				return
			}
		}
		if err = scanner.Err(); err != nil {
			return
		}
	}
	return
}

// Return informations from a TrueType font
func getInfoFromTrueType(fileStr string, msgWriter io.Writer, embed bool, encList encListType) (info fontType, err error) {
	ttf, err := TtfParse(fileStr)
	if err != nil {
		return info, err
	}
	if embed {
		if !ttf.Embeddable {
			err = fmt.Errorf("font license does not allow embedding")
			return
		}
		info.Data, err = ioutil.ReadFile(fileStr)
		if err != nil {
			return
		}
		info.OrigLen = len(info.Data)

		// Compress font for embedding
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		w.Write(info.Data)
		w.Close()
		info.Data = b.Bytes()
	}
	k := 1000.0 / float64(ttf.UnitsPerEm)
	info.Name = ttf.PostScriptName
	info.Bold = ttf.Bold
	info.Desc.ItalicAngle = int(ttf.ItalicAngle)
	info.IsFixedPitch = ttf.IsFixedPitch
	info.Desc.Ascent = round(k * float64(ttf.TypoAscender))
	info.Desc.Descent = round(k * float64(ttf.TypoDescender))
	info.Ut = round(k * float64(ttf.UnderlineThickness))
	info.Up = round(k * float64(ttf.UnderlinePosition))
	info.Desc.FontBBox = fontBoxType{
		round(k * float64(ttf.Xmin)),
		round(k * float64(ttf.Ymin)),
		round(k * float64(ttf.Xmax)),
		round(k * float64(ttf.Ymax)),
	}
	// printf("FontBBox\n")
	// dump(info.Desc.FontBBox)
	info.Desc.CapHeight = round(k * float64(ttf.CapHeight))
	info.Desc.MissingWidth = round(k * float64(ttf.Widths[0]))
	var wd int
	for j := 0; j < len(info.Cw); j++ {
		wd = info.Desc.MissingWidth
		if encList[j].name != ".notdef" {
			uv := encList[j].uv
			pos, ok := ttf.Chars[uint16(uv)]
			if ok {
				wd = round(k * float64(ttf.Widths[pos]))
			} else {
				fmt.Fprintf(msgWriter, "Character %s is missing\n", encList[j].name)
			}
		}
		info.Cw[j] = wd
	}
	if info.Desc.CapHeight == 0 {
		info.Desc.CapHeight = info.Desc.Ascent
	}
	info.Desc.Flags = 1 << 5
	if info.IsFixedPitch {
		info.Desc.Flags |= 1
	}
	if info.Desc.ItalicAngle != 0 {
		info.Desc.Flags |= 1 << 6
	}
	return
}

/*
   FONT UTILITIES
*/

// SetFontSize defines the size of the current font. Size is specified in
// points (1/ 72 inch). See also SetFontUnitSize().
func (f *Fpdf) SetFontSize(size float64) {
	if f.fontSizePt == size {
		return
	}
	f.fontSizePt = size
	f.fontSize = size / f.k
	if f.page > 0 {
		f.outf("BT /F%d %.2f Tf ET", f.currentFont.I, f.fontSizePt)
	}
}

// SetFontUnitSize defines the size of the current font. Size is specified in
// the unit of measure specified in New(). See also SetFontSize().
func (f *Fpdf) SetFontUnitSize(size float64) {
	if f.fontSize == size {
		return
	}
	f.fontSizePt = size * f.k
	f.fontSize = size
	if f.page > 0 {
		f.outf("BT /F%d %.2f Tf ET", f.currentFont.I, f.fontSizePt)
	}
}

// GetFontSize returns the size of the current font in points followed by the
// size in the unit of measure specified in New(). The second value can be used
// as a line height value in drawing operations.
func (f *Fpdf) GetFontSize() (ptSize, unitSize float64) {
	return f.fontSizePt, f.fontSize
}
