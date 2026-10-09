package keeldesktop

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var svgNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)

// The native SVG reader does not handle adjacent arc flags in optimized paths
// (for example "a4 4 0 012.3-.4"). Expand the SVG grammar's command groups and
// flag separators without changing coordinates or the source artwork.
func expandSVGPath(path string) (string, error) {
	var out strings.Builder
	var command byte
	for {
		path = strings.TrimLeft(path, " \t\r\n,")
		if path == "" {
			return out.String(), nil
		}
		if strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(path[0])) {
			command, path = path[0], path[1:]
		}
		upper := command &^ 32
		count := map[byte]int{'M': 2, 'L': 2, 'T': 2, 'H': 1, 'V': 1, 'C': 6, 'S': 4, 'Q': 4, 'A': 7, 'Z': 0}
		n, ok := count[upper]
		if !ok || command == 0 {
			return "", fmt.Errorf("invalid SVG path command %q", command)
		}
		out.WriteByte(command)
		for i := 0; i < n; i++ {
			path = strings.TrimLeft(path, " \t\r\n,")
			var token string
			if upper == 'A' && (i == 3 || i == 4) {
				if len(path) > 0 && (path[0] == '0' || path[0] == '1') {
					token = path[:1]
				}
			} else {
				token = svgNumber.FindString(path)
			}
			if token == "" {
				return "", fmt.Errorf("missing SVG %c parameter %d", command, i+1)
			}
			out.WriteByte(' ')
			out.WriteString(token)
			path = path[len(token):]
		}
		out.WriteByte(' ')
		if upper == 'M' {
			// Subsequent coordinate pairs after moveto are implicit lineto.
			command--
		}
		if upper == 'Z' {
			command = 0
		}
	}
}

func nativeSVG(data []byte) ([]byte, error) {
	reader := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	writer := xml.NewEncoder(&buf)
	for {
		token, err := reader.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "path" {
			for i := range start.Attr {
				if start.Attr[i].Name.Local == "d" {
					start.Attr[i].Value, err = expandSVGPath(start.Attr[i].Value)
					if err != nil {
						return nil, err
					}
				}
			}
			token = start
		}
		if err := writer.EncodeToken(token); err != nil {
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
