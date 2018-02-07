package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	//"strconv"
	"strings"
)

import (
	"golang.org/x/net/html"
	//"golang.org/x/text/encoding"
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

func noEof(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err != nil {
		if err == io.EOF {
			return false
		}
		panic(err)
	}
	return true
}

func parseContentType(ct string) (mime string, charset string) {
	ctv := strings.Split(ct, ";")
	for i, el := range ctv {
		ctv[i] = strings.ToLower(strings.TrimSpace(el))
	}

	mime = ctv[0]

	for i := 1; i < len(ctv); i++ {
		param := strings.SplitN(ctv[i], "=", 2)
		if len(param) != 2 {
			continue
		}

		key, val := param[0], param[1]
		if key != "charset" {
			continue
		}
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1:len(val)-1]
		}
		charset = val
	}

	return
}

func peekContentType(r *bufio.Reader) (mime string, charset string, source string) {
	buf, err := r.Peek(1024)
	if err != nil && err != io.EOF {
		panic(err)
	}

	z := html.NewTokenizer(bytes.NewReader(buf))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return "", "", "peekErr"
		}

		if tt != html.SelfClosingTagToken {
			continue
		}

		name, hasAttr := z.TagName()
		if hasAttr && bytes.Equal(name, []byte("meta")) {
			raw := string(z.Raw()[:])
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
				val, ok := attrs["charset"]
				if ok {
					return "text/html", val, raw
				}
			case 2:
				val, ok := attrs["content"]
				if attrs["http-equiv"] == "Content-Type" && ok {
					mime, charset := parseContentType(val)
					return mime, charset, raw
				}
			}
		}
	}
	return "", "", "peekNone"
}

func matchesCriteria(r io.Reader, uri string) bool {
	// FIXME: it's now double buffered, maybe use NewReaderSize to make it more sensible?
	br := bufio.NewReader(r)

	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		panic(err)
	}

	mime, charset := "", ""
	source := ""

	ct := resp.Header["Content-Type"]
	// FIXME: what about XHTML?
	if ct != nil {
		mime, charset = parseContentType(ct[0])
		if mime != "text/html" {
			return false
		}
		source = ct[0]
	}

	if charset == "" {
		// FIXME: decide wheter to use this encoding
		mime, charset, source = peekContentType(br)
		if mime != "text/html" {
			return false
		}
	}
	if charset == "" {
		charset = "iso-8859-1"
		source = "default"
	}

	enc, err := htmlindex.Get(charset)
	if err != nil {
		fmt.Printf("%v: %s %q (%q)\n", err, uri, charset, source)
		return false
	}

	z := html.NewTokenizer(enc.NewDecoder().Reader(br))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			//fmt.Printf("%s: error\n", warc_target_uri)
			return false
		}

		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, _ := z.TagName()
			if bytes.Equal(name, []byte("script")) {
				//fmt.Printf("%s: %q\n", warc_target_uri, z.Raw())
				return false
			}
		}
	}
	return true
}

func main() {
	r := bufio.NewReader(os.Stdin)

	for ; noEof(r); {
		line := getLine(r)
		if !bytes.Equal(line, []byte("WARC/1.0")) {
			panic(fmt.Sprintf("expected WARC/1.0, instead: %q", line))
		}

		warc_type_response := false
		var warc_content_length uint = 0
		var warc_target_uri []byte
		var warc_truncated []byte

		for ;; {
			line := getLine(r)
			if len(line) == 0 {
				// end of the header
				break
			}
			field := bytes.SplitN(line, []byte(": "), 2)
			if bytes.Equal(field[0], []byte("WARC-Type")) {
				if bytes.Equal(field[1], []byte("response")) {
					warc_type_response = true
				}
			} else if bytes.Equal(field[0], []byte("Content-Length")) {
				n, err := fmt.Sscanf(string(field[1]), "%v", &warc_content_length);
				if err != nil {
					panic(err)
				}
				if n != 1 {
					panic(fmt.Sprintf("Content-Length: expected integer, instead: %q", field[1]))
				}
			} else if bytes.Equal(field[0], []byte("WARC-Target-URI")) {
				warc_target_uri = field[1]
			} else if bytes.Equal(field[0], []byte("WARC-Truncated")) {
				warc_truncated = field[1]
			}
		}

		if warc_content_length == 0 {
			panic("expected Content-Length > 0")
		}

		lr := io.LimitedReader{r, int64(warc_content_length)}
		
		if warc_type_response && matchesCriteria(&lr, string(warc_target_uri)) {
			fmt.Printf("%s %v %s\n", warc_target_uri, warc_content_length, warc_truncated)
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

