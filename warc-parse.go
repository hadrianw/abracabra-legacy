package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	//"strconv"
	//"strings"
)

import (
	"golang.org/x/net/html"
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
		
		if warc_type_response {
			// FIXME: it's now double buffered, maybe use NewReaderSize to make it more sensible?
			blr := bufio.NewReader(&lr)

			resp, err := http.ReadResponse(blr, nil)
			if err != nil {
				panic(err)
			}

			// FIXME: check for Content-Type html

			// FIXME: make sure that we have UTF-8
			accept := true
			z := html.NewTokenizer(blr)
			for {
				tt := z.Next()
				if tt == html.ErrorToken {
					//fmt.Printf("%s: error\n", warc_target_uri)
					accept = false
					break
				}

				if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
					name, _ := z.TagName()
					if bytes.Equal(name, []byte("script")) {
						//fmt.Printf("%s: %q\n", warc_target_uri, z.Raw())
						accept = false
						break
					}
				}
			}

			if accept {
				fmt.Printf("%s %v %s\n", warc_target_uri, warc_content_length, warc_truncated)
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

