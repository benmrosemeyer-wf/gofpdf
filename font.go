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
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AddFont imports a TrueType, OpenType or Type1 font and makes it available.
// It is necessary to generate a font definition file first with the makefont
// utility. It is not necessary to call this function for the core PDF fonts
// (courier, helvetica, times, zapfdingbats).
//
// The JSON definition file (and the font file itself when embedding) must be
// present in the font directory. If it is not found, the error "Could not
// include font definition file" is set.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// fileStr specifies the base name with ".json" extension of the font
// definition file to be added. The file will be loaded from the font directory
// specified in the call to New() or SetFontLocation().
func (f *Fpdf) AddFont(familyStr, styleStr, fileStr string) {
	if fileStr == "" {
		fileStr = strings.Replace(familyStr, " ", "", -1) + strings.ToLower(styleStr) + ".ttf"
	}

	if f.fontLoader != nil {
		reader, err := f.fontLoader.Open(fileStr)
		if err == nil {
			f.AddFontFromReader(familyStr, styleStr, reader)
			if closer, ok := reader.(io.Closer); ok {
				closer.Close()
			}
			return
		}
	}

	fileStr = path.Join(f.fontpath, fileStr)
	file, err := os.Open(fileStr)
	if err != nil {
		f.err = err
		return
	}
	defer file.Close()

	f.AddFontFromReader(familyStr, styleStr, file)
}

// AddTTFFont does the same as AddFont, but doesn't require .json or .z files
func (f *Fpdf) AddTTFFont(familyStr, styleStr, fileStr string) {
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
	makeFontDescriptor(&info)
	info.OriginalSize = len(info.Data) // TODO: try and get rid of OriginalSize

	// TODO: cache compressed data! (the stupid *.z files)
	info.File = FileStr

	f.addFontFromInfo(info, fontkey)
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

// AddFontFromReader imports a TrueType, OpenType or Type1 font and makes it
// available using a reader that satisifies the io.Reader interface. See
// AddFont for details about familyStr and styleStr.
func (f *Fpdf) AddFontFromReader(familyStr, styleStr string, r io.Reader) {
	if f.err != nil {
		return
	}
	// dbg("Adding family [%s], style [%s]", familyStr, styleStr)
	var ok bool
	fontkey := getFontKey(familyStr, styleStr)
	_, ok = f.fonts[fontkey]
	if ok {
		return
	}
	var info fontType
	info = f.loadfont(r)
	if f.err != nil {
		return
	}
	f.addFontFromInfo(info, fontkey)
}

func (f *Fpdf) addFontFromInfo(info fontType, fontkey string) {
	info.I = len(f.fonts)
	if len(info.Diff) > 0 {
		// Search existing encodings
		n := -1
		for j, str := range f.diffs {
			if str == info.Diff {
				n = j + 1
				break
			}
		}
		if n < 0 {
			f.diffs = append(f.diffs, info.Diff)
			n = len(f.diffs)
		}
		info.DiffN = n
	}
	// dbg("font [%s], type [%s]", info.File, info.Tp)
	f.fonts[fontkey] = info
	return
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
	var ok bool
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
	_, ok = f.fonts[fontkey]
	if !ok {
		// Test if one of the core fonts
		if familyStr == "arial" {
			familyStr = "helvetica"
		}
		_, ok = f.coreFonts[familyStr]
		if ok {
			if familyStr == "symbol" {
				familyStr = "zapfdingbats"
			}
			if familyStr == "zapfdingbats" {
				styleStr = ""
			}
			fontkey = familyStr + styleStr
			_, ok = f.fonts[fontkey]
			if !ok {
				rdr := f.coreFontReader(familyStr, styleStr)
				if f.err == nil {
					f.AddFontFromReader(familyStr, styleStr, rdr)
				}
				if f.err != nil {
					return
				}
			}
		} else {
			f.err = fmt.Errorf("undefined font: %s %s", familyStr, styleStr)
			return
		}
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
func (f *Fpdf) putfonts() {
	if f.err != nil {
		return
	}
	nf := f.n
	for _, diff := range f.diffs {
		// Encodings
		f.newobj()
		f.outf("<</Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [%s]>>", diff)
		f.out("endobj")
	}
	{
		var fileList []string
		lookup := make(map[string]fontType)
		for _, info := range f.fonts {
			if len(info.File) > 0 {
				fileList = append(fileList, info.File)
				lookup[info.File] = info
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
			// TODO: load cached and gzipped font data from fontDesc
			font, err := f.loadFontFile(info.File)
			if err != nil {
				f.err = err
				return
			}
			// dbg("font file [%s], ext [%s]", file, file[len(file)-2:])
			compressed := info.File[len(info.File)-2:] == ".z"
			length := info.OriginalSize
			// fmt.Printf("%s: legnth at start: %d; Data: %d\n", info.File, length, len(info.Data))
			if !compressed && info.Size2 > 0 {
				length = info.Size1
				buf := font[6:info.Size1]
				buf = append(buf, font[6+info.Size1+6:info.Size2]...)
				font = buf
			}
			f.outf("<</Length %d", len(font))
			if compressed {
				f.out("/Filter /FlateDecode")
			}
			f.outf("/Length1 %d", length)
			if info.Size2 > 0 {
				f.outf("/Length2 %d /Length3 0", info.Size2)
			}
			f.out(">>")
			f.putstream(font)
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
			tp := font.Tp
			name := font.Name
			if tp == "Core" {
				// Core font
				f.newobj()
				f.out("<</Type /Font")
				f.outf("/BaseFont /%s", name)
				f.out("/Subtype /Type1")
				if name != "Symbol" && name != "ZapfDingbats" {
					f.out("/Encoding /WinAnsiEncoding")
				}
				f.out(">>")
				f.out("endobj")
			} else if tp == "Type1" || tp == "TrueType" {
				// Additional Type1 or TrueType/OpenType font
				f.newobj()
				f.out("<</Type /Font")
				f.outf("/BaseFont /%s", name)
				f.outf("/Subtype /%s", tp)
				f.out("/FirstChar 32 /LastChar 255")
				f.outf("/Widths %d 0 R", f.n+1)
				f.outf("/FontDescriptor %d 0 R", f.n+2)
				if font.DiffN > 0 {
					f.outf("/Encoding %d 0 R", nf+font.DiffN)
				} else {
					f.out("/Encoding /WinAnsiEncoding")
				}
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
				s.printf("/StemV %d ", font.Desc.StemV)
				s.printf("/MissingWidth %d ", font.Desc.MissingWidth)
				var suffix string
				if tp != "Type1" {
					suffix = "2"
				}
				s.printf("/FontFile%s %d 0 R>>", suffix, origN)
				f.out(s.String())
				f.out("endobj")
			} else {
				f.err = fmt.Errorf("unsupported font type: %s", tp)
				return
				// Allow for additional types
				// 			$mtd = 'put'.strtolower($type);
				// 			if(!method_exists($this,$mtd))
				// 				$this->Error('Unsupported font type: '.$type);
				// 			$this->$mtd($font);
			}
		}
	}
	return
}

func (f *Fpdf) loadFontFile(name string) ([]byte, error) {
	if f.fontLoader != nil {
		reader, err := f.fontLoader.Open(name)
		if err == nil {
			data, err := ioutil.ReadAll(reader)
			if closer, ok := reader.(io.Closer); ok {
				closer.Close()
			}
			return data, err
		}
	}
	return ioutil.ReadFile(path.Join(f.fontpath, name))
}

func baseNoExt(fileStr string) string {
	str := filepath.Base(fileStr)
	extLen := len(filepath.Ext(str))
	if extLen > 0 {
		str = str[:len(str)-extLen]
	}
	return str
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
	var ttf TtfType
	ttf, err = TtfParse(fileStr)
	if err != nil {
		return
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
	// printf("getInfoFromTrueType/FontBBox\n")
	// dump(info.Desc.FontBBox)
	return
}

type segmentType struct {
	marker uint8
	tp     uint8
	size   uint32
	data   []byte
}

func segmentRead(f *os.File) (s segmentType, err error) {
	if err = binary.Read(f, binary.LittleEndian, &s.marker); err != nil {
		return
	}
	if s.marker != 128 {
		err = fmt.Errorf("font file is not a valid binary Type1")
		return
	}
	if err = binary.Read(f, binary.LittleEndian, &s.tp); err != nil {
		return
	}
	if err = binary.Read(f, binary.LittleEndian, &s.size); err != nil {
		return
	}
	s.data = make([]byte, s.size)
	_, err = f.Read(s.data)
	return
}

// -rw-r--r-- 1 root root  9532 2010-04-22 11:27 /usr/share/fonts/type1/mathml/Symbol.afm
// -rw-r--r-- 1 root root 37744 2010-04-22 11:27 /usr/share/fonts/type1/mathml/Symbol.pfb

// Return informations from a Type1 font
func getInfoFromType1(fileStr string, msgWriter io.Writer, embed bool, encList encListType) (info fontType, err error) {
	if embed {
		var f *os.File
		f, err = os.Open(fileStr)
		if err != nil {
			return
		}
		defer f.Close()
		// Read first segment
		var s1, s2 segmentType
		s1, err = segmentRead(f)
		if err != nil {
			return
		}
		s2, err = segmentRead(f)
		if err != nil {
			return
		}
		info.Data = s1.data
		info.Data = append(info.Data, s2.data...)
		info.Size1 = int(s1.size)
		info.Size2 = int(s2.size)
	}
	afmFileStr := fileStr[0:len(fileStr)-3] + "afm"
	size, ok := fileSize(afmFileStr)
	if !ok {
		err = fmt.Errorf("font file (ATM) %s not found", afmFileStr)
		return
	} else if size == 0 {
		err = fmt.Errorf("font file (AFM) %s empty or not readable", afmFileStr)
		return
	}
	var f *os.File
	f, err = os.Open(afmFileStr)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var fields []string
	var wd int
	var wt, name string
	wdMap := make(map[string]int)
	for scanner.Scan() {
		fields = strings.Fields(strings.TrimSpace(scanner.Text()))
		// Comment Generated by FontForge 20080203
		// FontName Symbol
		// C 32 ; WX 250 ; N space ; B 0 0 0 0 ;
		if len(fields) >= 2 {
			switch fields[0] {
			case "C":
				if wd, err = strconv.Atoi(fields[4]); err == nil {
					name = fields[7]
					wdMap[name] = wd
				}
			case "FontName":
				info.Name = fields[1]
			case "Weight":
				wt = strings.ToLower(fields[1])
			case "ItalicAngle":
				info.Desc.ItalicAngle, err = strconv.Atoi(fields[1])
			case "Ascender":
				info.Desc.Ascent, err = strconv.Atoi(fields[1])
			case "Descender":
				info.Desc.Descent, err = strconv.Atoi(fields[1])
			case "UnderlineThickness":
				info.Ut, err = strconv.Atoi(fields[1])
			case "UnderlinePosition":
				info.Up, err = strconv.Atoi(fields[1])
			case "IsFixedPitch":
				info.IsFixedPitch = fields[1] == "true"
			case "FontBBox":
				if info.Desc.FontBBox.Xmin, err = strconv.Atoi(fields[1]); err == nil {
					if info.Desc.FontBBox.Ymin, err = strconv.Atoi(fields[2]); err == nil {
						if info.Desc.FontBBox.Xmax, err = strconv.Atoi(fields[3]); err == nil {
							info.Desc.FontBBox.Ymax, err = strconv.Atoi(fields[4])
						}
					}
				}
			case "CapHeight":
				info.Desc.CapHeight, err = strconv.Atoi(fields[1])
			case "StdVW":
				info.Desc.StemV, err = strconv.Atoi(fields[1])
			}
		}
		if err != nil {
			return
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	if info.Name == "" {
		err = fmt.Errorf("the field FontName missing in AFM file %s", afmFileStr)
		return
	}
	info.Bold = wt == "bold" || wt == "black"
	var missingWd int
	missingWd, ok = wdMap[".notdef"]
	if ok {
		info.Desc.MissingWidth = missingWd
	}
	for j := 0; j < len(info.Cw); j++ {
		info.Cw[j] = info.Desc.MissingWidth
	}
	for j := 0; j < len(info.Cw); j++ {
		name = encList[j].name
		if name != ".notdef" {
			wd, ok = wdMap[name]
			if ok {
				info.Cw[j] = wd
			} else {
				fmt.Fprintf(msgWriter, "Character %s is missing\n", name)
			}
		}
	}
	// printf("getInfoFromType1/FontBBox\n")
	// dump(info.Desc.FontBBox)
	return
}

func makeFontDescriptor(info *fontType) {
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
	if info.Desc.StemV == 0 {
		if info.Bold {
			info.Desc.StemV = 120
		} else {
			info.Desc.StemV = 70
		}
	}
	// printf("makeFontDescriptor/FontBBox\n")
	// dump(info.Desc.FontBBox)
}

// Build differences from reference encoding
func makeFontEncoding(encList encListType, refEncFileStr string) (diffStr string, err error) {
	var refList encListType
	if refList, err = loadMap(refEncFileStr); err != nil {
		return
	}
	var buf fmtBuffer
	last := 0
	for j := 32; j < 256; j++ {
		if encList[j].name != refList[j].name {
			if j != last+1 {
				buf.printf("%d ", j)
			}
			last = j
			buf.printf("/%s ", encList[j].name)
		}
	}
	diffStr = strings.TrimSpace(buf.String())
	return
}

func makeDefinitionFile(fileStr, encodingFileStr string, embed bool, encList encListType, info fontType) (err error) {
	makeFontDescriptor(&info)
	info.Enc = baseNoExt(encodingFileStr)
	// fmt.Printf("encodingFileStr [%s], def.Enc [%s]\n", encodingFileStr, def.Enc)
	// fmt.Printf("reference [%s]\n", filepath.Join(filepath.Dir(encodingFileStr), "cp1252.map"))
	info.Diff, err = makeFontEncoding(encList, filepath.Join(filepath.Dir(encodingFileStr), "cp1252.map"))
	if err != nil {
		return
	}
	info.OriginalSize = len(info.Data)
	// printf("Font definition file [%s]\n", fileStr)
	var buf []byte
	buf, err = json.Marshal(info)
	if err != nil {
		return
	}
	var f *os.File
	f, err = os.Create(fileStr)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(buf)
	return
}

// MakeFont generates a font definition file in JSON format. A definition file
// of this type is required to use non-core fonts in the PDF documents that
// gofpdf generates. See the makefont utility in the gofpdf package for a
// command line interface to this function.
//
// fontFileStr is the name of the TrueType file (extension .ttf), OpenType file
// (extension .otf) or binary Type1 file (extension .pfb) from which to
// generate a definition file. If an OpenType file is specified, it must be one
// that is based on TrueType outlines, not PostScript outlines; this cannot be
// determined from the file extension alone. If a Type1 file is specified, a
// metric file with the same pathname except with the extension .afm must be
// present.
//
// encodingFileStr is the name of the encoding file that corresponds to the
// font.
//
// dstDirStr is the name of the directory in which to save the definition file
// and, if embed is true, the compressed font file.
//
// msgWriter is the writer that is called to display messages throughout the
// process. Use nil to turn off messages.
//
// embed is true if the font is to be embedded in the PDF files.
func MakeFont(fontFileStr, encodingFileStr, dstDirStr string, msgWriter io.Writer, embed bool) (err error) {
	if msgWriter == nil {
		msgWriter = ioutil.Discard
	}
	if !fileExist(fontFileStr) {
		err = fmt.Errorf("font file not found: %s", fontFileStr)
		return
	}
	extStr := strings.ToLower(fontFileStr[len(fontFileStr)-3:])
	// printf("Font file extension [%s]\n", extStr)
	var tpStr string
	if extStr == "ttf" || extStr == "otf" {
		tpStr = "TrueType"
	} else if extStr == "pfb" {
		tpStr = "Type1"
	} else {
		err = fmt.Errorf("unrecognized font file extension: %s", extStr)
		return
	}
	var encList encListType
	var info fontType
	encList, err = loadMap(encodingFileStr)
	if err != nil {
		return
	}
	// printf("Encoding table\n")
	// dump(encList)
	if tpStr == "TrueType" {
		info, err = getInfoFromTrueType(fontFileStr, msgWriter, embed, encList)
		if err != nil {
			return
		}
	} else {
		info, err = getInfoFromType1(fontFileStr, msgWriter, embed, encList)
		if err != nil {
			return
		}
	}
	info.Tp = tpStr
	baseStr := baseNoExt(fontFileStr)
	// fmt.Printf("Base [%s]\n", baseStr)
	if embed {
		var f *os.File
		info.File = baseStr + ".z"
		zFileStr := filepath.Join(dstDirStr, info.File)
		f, err = os.Create(zFileStr)
		if err != nil {
			return
		}
		defer f.Close()
		cmp := zlib.NewWriter(f)
		cmp.Write(info.Data)
		cmp.Close()
		fmt.Fprintf(msgWriter, "Font file compressed: %s\n", zFileStr)
	}
	defFileStr := filepath.Join(dstDirStr, baseStr+".json")
	err = makeDefinitionFile(defFileStr, encodingFileStr, embed, encList, info)
	if err != nil {
		return
	}
	fmt.Fprintf(msgWriter, "Font definition file successfully generated: %s\n", defFileStr)
	return
}
