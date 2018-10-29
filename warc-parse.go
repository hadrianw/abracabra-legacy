package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	//"strconv"
	//"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
)

func getLine(r *bufio.Reader) []byte {
	line, err := r.ReadBytes('\r')
	if err != nil {
		panic(err)
	}
	lf, err := r.ReadByte()
	if err != nil {
		panic(err)
	}
	if lf != byte('\n') {
		panic(fmt.Sprintf("expected LF after CR, instead: %q", lf))
	}
	// remove CR
	return line[:len(line)-1]
}

func noEOF(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err != nil {
		if err == io.EOF {
			return false
		}
		panic(err)
	}
	return true
}

func parseMediaType(contentType string) (mediatype string, charset string) {
	mediatype, params, err := mime.ParseMediaType(contentType)
	if err == nil {
		charset = params["charset"]
	}

	return
}

func determineEncoding(r *bufio.Reader, contentType string) (mediatype string, enc encoding.Encoding) {
	buf, err := r.Peek(1024)
	if err != nil && err != io.EOF {
		panic(err)
	}

	enc, _, certain := charset.DetermineEncoding(buf, contentType)

	charset := ""
	z := html.NewTokenizer(bytes.NewReader(buf))
	for {
		tt := z.Next()
		if tt == html.DoctypeToken {
			// FIXME: handle XHTML and friends?
			mediatype = "text/html"
			continue
		} else if tt == html.ErrorToken {
			// FIXME: reports errors except EOF
			break
		} else if tt != html.SelfClosingTagToken {
			continue
		}

		name, hasAttr := z.TagName()
		if hasAttr && bytes.Equal(name, []byte("meta")) {
			//raw := string(z.Raw()[:])
			moreAttr := true
			attrs := make(map[string]string)
			for moreAttr {
				var key, val []byte
				key, val, moreAttr = z.TagAttr()
				if bytes.Equal(key, []byte("charset")) ||
					bytes.Equal(key, []byte("http-equiv")) ||
					bytes.Equal(key, []byte("content")) {
					attrs[string(key)] = string(val)
				}
			}
			switch len(attrs) {
			case 1:
				var ok bool
				charset, ok = attrs["charset"]
				if ok {
					mediatype = "text/html"
					break
				}
			case 2:
				val, ok := attrs["content"]
				if ok && attrs["http-equiv"] == "Content-Type" {
					mediatype, charset = parseMediaType(val)
					break
				}
			}
		}
	}
	if !certain && charset != "" {
		metaEnc, err := htmlindex.Get(charset)
		if err != nil {
			return
		}
		enc = metaEnc
	}
	return
}

func uriFilter(uri string) bool {

}

func blacklistFilter(uri string) bool {

}

func check(r io.Reader, uri string) (ads bool, code bool, err error) {
	ads = false
	// js, object or embed
	code = false

	ads = blacklistFilter(uri)
	if ads {
		return
	}

	// FIXME: it's now double buffered, maybe use NewReaderSize to make it more sensible?
	br := bufio.NewReader(r)

	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return
	}

	contentType, mediatype := "", ""
	//source := ""

	// FIXME: what about XHTML?
	if ct := resp.Header["Content-Type"]; ct != nil {
		contentType = ct[0]
		mediatype, _ = parseMediaType(contentType)
		if mediatype != "text/html" {
			// FIXME: what if empty?
			err = errors.New("Unsupported mediatype " + mediatype)
			return
		}
		//source = contentType
	}

	// FIXME: decide wheter to use this encoding
	mediatype, enc := determineEncoding(br, contentType)
	if mediatype != "text/html" {
		err = errors.New("Unsupported mediatype " + mediatype)
		return
	}

	z := html.NewTokenizer(enc.NewDecoder().Reader(br))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				err = z.Err()
			}
			return
		}

		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, moreAttr := z.TagName()
			nameStr := string(name)

			img := false
			urlAttr := ""

			switch nameStr {
			case "img":
				urlAttr = "src"
				img = true
			case "script", "embed":
				urlAttr = "src"
				code = true
			case "object":
				urlAttr = "data"
				code = true
			case "iframe", "video", "audio", "source", "track":
				urlAttr = "src"
			case "link":
				urlAttr = "href"
			}

			for moreAttr {
				key, val, moreAttr = z.TagAttr()
				if img && bytes.Equal(key, []byte("imgset") {
					for srcset := bytes.NewBuffer(val);
					src, err := srcset.ReadBytes(' ');
					_, err := srcset.ReadBytes(',') {
						ads = uriFilter(src)
						if ads {
							return
						}
					}
				} else if urlAttr == string(key) {
					ads = uriFilter(val)
					if ads {
						return
					}
				} else if bytes.Equal(key, []byte("class") || bytes.Equal(key, []byte("id") {

				} else if bytes.HasPrefix(key, []byte("on")) {
					code = true
				}
			}
		}
	}
}

/*
does filtering on a/href makes sense?
tag attribs:

* on* - js
link href
audio src
	source src
img src,srcset
video src
	track src
embed src
iframe src
script src
object data
	also: type for possible code execution
*/

func main() {
	r := bufio.NewReader(os.Stdin)

	for noEOF(r) {
		line := getLine(r)
		if !bytes.Equal(line, []byte("WARC/1.0")) {
			panic(fmt.Sprintf("expected WARC/1.0, instead: %q", line))
		}

		warcTypeResponse := false
		var warcContentLength uint
		var warcTargetURI []byte
		var warcTruncated []byte

		for {
			line := getLine(r)
			if len(line) == 0 {
				// end of the header
				break
			}
			field := bytes.SplitN(line, []byte(": "), 2)
			if bytes.Equal(field[0], []byte("WARC-Type")) {
				if bytes.Equal(field[1], []byte("response")) {
					warcTypeResponse = true
				}
			} else if bytes.Equal(field[0], []byte("Content-Length")) {
				n, err := fmt.Sscanf(string(field[1]), "%v", &warcContentLength)
				if err != nil {
					panic(err)
				}
				if n != 1 {
					panic(fmt.Sprintf("Content-Length: expected integer, instead: %q", field[1]))
				}
			} else if bytes.Equal(field[0], []byte("WARC-Target-URI")) {
				warcTargetURI = field[1]
			} else if bytes.Equal(field[0], []byte("WARC-Truncated")) {
				warcTruncated = field[1]
			}
		}

		if warcContentLength == 0 {
			panic("expected Content-Length > 0")
		}

		lr := io.LimitedReader{R: r, N: int64(warcContentLength)}

		if warcTypeResponse {
			ads, code, err := check(&lr, string(warcTargetURI));
			if err {
				fmt.Fprintf(os.Stderr, "check error: %s", err);
			} else if  !ads {
				fmt.Printf("%v %t %s %s %s\n", warcContentLength, code, warcTargetURI, warcTruncated)
			}
		}
		r.Discard(int(lr.N))

		var sep [4]byte
		_, err := io.ReadFull(r, sep[:])
		if err != nil {
			panic(err)
		}
		if !bytes.Equal(sep[:], []byte("\r\n\r\n")) {
			panic(fmt.Sprintf("expected record separator '\\r\\n\\r\\n', instead: %q", sep))
		}
	}
}
